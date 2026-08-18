// Пакет toolchain инкапсулирует механизм скачивания и хранения внешних
// инструментов (go, php, lnav, линтеры) в папке ~/dev-config/tools.
//
// Каждая программа описывается структурой Program. Разные версии одной
// программы хранятся в разных подпапках, поэтому версии не пересекаются.
//
// Скачивание выполняется лениво: фабрики (PhpProgram, GoProgram, LnavProgram)
// не обращаются к сети. Сеть используется только в Manager.Ensure, когда
// нужной версии нет в локальной папке инструментов.
package toolchain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirName — имя подпапки внутри dev-config, куда скачиваются программы.
const DirName = "tools"

// Program описывает одну программу: рантайм (go, php), просмотрщик (lnav)
// или линтер.
type Program struct {
	// Name — короткое имя программы, используется в плейсхолдерах команд
	// (например {go}, {php}, {phpstan}).
	Name string
	// Version — требуемая версия программы. Определяет подпапку хранения
	// (<name>/<version>), поэтому разные версии не пересекаются.
	Version string
	// Binary — относительный путь к исполняемому файлу внутри подпапки
	// программы. Для программ, URL которых определяется динамически (php, go),
	// Binary известен без сети.
	Binary string
	// URL — адрес скачивания. Для программ с динамически определяемым URL
	// (php, go) заполняется только при резолюции в Ensure. Для остальных
	// задаётся фабрикой.
	URL string
	// Archive — тип архива: "" (простой файл), "tar.gz", "tar.xz", "zip".
	// Заполняется так же, как URL.
	Archive string
	// FullCommand — команда для запуска программы. Плейсхолдеры вида {имя}
	// заменяются на полные пути соответствующих программ из набора.
	FullCommand string
	// Require — зависимости программы (программы, которые должны быть скачаны
	// до запуска данной).
	Require []Program
}

// Manager управляет загрузкой и хранением инструментов.
type Manager struct {
	// dir — абсолютный путь к папке, куда скачиваются программы.
	dir string
}

// NewManager создаёт менеджер инструментов с папкой ~/dev-config/tools.
// Возвращает ошибку, если домашнюю директорию определить не удалось.
func NewManager() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine home directory: %v", err)
	}
	return &Manager{
		dir: filepath.Join(home, "dev-config", DirName),
	}, nil
}

// Dir возвращает абсолютный путь к папке хранения инструментов.
func (m *Manager) Dir() string {
	return m.dir
}

// programDir возвращает путь к папке, в которую распаковывается программа.
// Путь строится как <папка-инструментов>/<name>/<version>
func (m *Manager) programDir(p Program) string {
	return filepath.Join(m.dir, p.Name, p.Version)
}

// BinaryPath возвращает полный путь к исполняемому файлу программы.
func (m *Manager) BinaryPath(p Program) string {
	return filepath.Join(m.programDir(p), p.Binary)
}

// IsInstalled проверяет, существует ли исполняемый файл программы нужной версии.
func (m *Manager) IsInstalled(p Program) bool {
	_, err := os.Stat(m.BinaryPath(p))
	return err == nil
}

// Command строит имя команды и список аргументов для запуска программы,
// заменяя плейсхолдеры {имя} на полные пути бинарей всех программ набора.
func (m *Manager) Command(p Program, args []string) (string, []string) {
	full := p.FullCommand
	// Подставляем саму программу.
	full = strings.ReplaceAll(full, "{"+p.Name+"}", m.BinaryPath(p))
	// Подставляем зависимости (Require) программы.
	for _, v := range p.Require {
		full = strings.ReplaceAll(full, "{"+v.Name+"}", m.BinaryPath(v))
	}
	parts := strings.Fields(full)
	if len(parts) == 0 {
		parts = []string{m.BinaryPath(p)}
	}
	parts = append(parts, args...)
	return parts[0], parts[1:]
}

// expandPrograms разворачивает список программ с учётом их зависимостей
// (Require) в плоский список без дубликатов. Зависимости идут после
// программы, которая их требует.
func expandPrograms(programs []Program) []Program {
	var result []Program
	seen := make(map[string]bool)
	var walk func([]Program)
	walk = func(list []Program) {
		for _, p := range list {
			if seen[p.Name] {
				continue
			}
			seen[p.Name] = true
			result = append(result, p)
			walk(p.Require)
		}
	}
	walk(programs)
	return result
}

// Ensure гарантирует, что переданные программы и их зависимости доступны
// локально. Если нужной версии программы нет в папке инструментов, программа
// резолвится (определяется конкретный URL) и скачивается. Сеть используется
// только при отсутствии нужной версии.
func (m *Manager) Ensure(programs ...Program) ([]Program, error) {
	if err := os.MkdirAll(m.dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %v", m.dir, err)
	}
	flat := expandPrograms(programs)
	current := make(map[string]Program)
	for _, p := range flat {
		if m.IsInstalled(p) {
			current[p.Name] = p
			continue
		}
		resolved, err := m.resolve(p)
		if err != nil {
			return nil, err
		}
		if err := m.download(resolved); err != nil {
			return nil, err
		}
		current[p.Name] = resolved
	}
	return rebuildWithDeps(flat, current), nil
}

// rebuildWithDeps собирает итоговый список программ, заменяя каждую программу
// на актуальную установленную версию и обновляя зависимости (Require)
// на актуальные версии.
func rebuildWithDeps(flat []Program, current map[string]Program) []Program {
	result := make([]Program, 0, len(flat))
	for _, p := range flat {
		cp := current[p.Name]
		cp.Require = nil
		for _, dep := range p.Require {
			if act, ok := current[dep.Name]; ok {
				cp.Require = append(cp.Require, act)
			} else {
				cp.Require = append(cp.Require, dep)
			}
		}
		result = append(result, cp)
	}
	return result
}

// resolve определяет конкретный URL и тип архива для программы. Для php и go
// это требует обращения к сети (актуальная версия и адрес скачивания).
// Для остальных программ (lnav, линтеры) URL уже задан фабрикой — сеть не нужна.
func (m *Manager) resolve(p Program) (Program, error) {
	switch p.Name {
	case "php":
		return resolvePhp(p)
	case "go":
		return resolveGo(p)
	default:
		return p, nil
	}
}
