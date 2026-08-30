// Пакет toolchain инкапсулирует механизм скачивания и хранения внешних
// инструментов (go, php, lnav, линтеры) в папке ~/dev-config/tools.
//
// Разные версии одной программы хранятся в разных подпапках, поэтому версии
// не пересекаются. Скачивание выполняется лениво: фабрики (PhpProgram,
// GoProgram, LnavProgram) не обращаются к сети. Сеть используется только в
// Manager.Ensure, когда нужной версии нет ни в системе, ни в локальной папке.
package toolchain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirName — имя подпапки внутри dev-config, куда скачиваются программы.
const DirName = "tools"

// Program описывает программу с фиксированной версией (lnav, линтеры).
// Реализует Executable. Версие-логики не имеет — её нет в рантаймах.
type Program struct {
	name        string
	version     string
	binary      string
	url         string
	archive     string
	fullCommand string
	require     []Executable
}

// NewProgram создаёт описание программы с заданными параметрами.
func NewProgram(name, version, binary, url, archive, fullCommand string, require ...Executable) *Program {
	return &Program{
		name:        name,
		version:     version,
		binary:      binary,
		url:         url,
		archive:     archive,
		fullCommand: fullCommand,
		require:     require,
	}
}

func (p *Program) Name() string          { return p.name }
func (p *Program) Version() string       { return p.version }
func (p *Program) Binary() string        { return p.binary }
func (p *Program) URL() string           { return p.url }
func (p *Program) Archive() string       { return p.archive }
func (p *Program) FullCommand() string   { return p.fullCommand }
func (p *Program) Require() []Executable { return p.require }

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
// Путь строится как <папка-инструментов>/<name>/<version>.
func (m *Manager) programDir(ex Executable) string {
	return filepath.Join(m.dir, ex.Name(), ex.Version())
}

// BinaryPath возвращает полный путь к исполняемому файлу программы.
// Если у программы задан абсолютный путь к бинарю (системный рантайм из PATH),
// он возвращается как есть. Иначе путь строится внутри папки хранения.
func (m *Manager) BinaryPath(ex Executable) string {
	if filepath.IsAbs(ex.Binary()) {
		return ex.Binary()
	}
	return filepath.Join(m.programDir(ex), ex.Binary())
}

// IsInstalled проверяет, существует ли исполняемый файл программы нужной версии.
// Установка выполняется атомарно (см. download), поэтому наличие бинаря
// гарантирует, что версия полностью распакована и готова к запуску.
func (m *Manager) IsInstalled(ex Executable) bool {
	_, err := os.Stat(m.BinaryPath(ex))
	return err == nil
}

// replaceDir атомарно заменяет папку dst содержимым src: удаляет прежнее
// значение и перемещает src на место dst. Гарантирует, что dst всегда содержит
// либо полностью готовую программу, либо отсутствует вовсе (никогда — частично
// распакованную).
func replaceDir(dst, src string) error {
	if _, err := os.Lstat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}
	return os.Rename(src, dst)
}

// Command строит имя команды и список аргументов для запуска программы,
// заменяя плейсхолдеры {имя} на полные пути бинарей всех программ набора.
func (m *Manager) Command(ex Executable, args []string) (string, []string) {
	full := ex.FullCommand()
	// Подставляем саму программу.
	full = strings.ReplaceAll(full, "{"+ex.Name()+"}", m.BinaryPath(ex))
	// Подставляем зависимости (Require) программы.
	for _, v := range ex.Require() {
		full = strings.ReplaceAll(full, "{"+v.Name()+"}", m.BinaryPath(v))
	}
	parts := strings.Fields(full)
	if len(parts) == 0 {
		parts = []string{m.BinaryPath(ex)}
	}
	parts = append(parts, args...)
	return parts[0], parts[1:]
}

// expandPrograms разворачивает список программ с учётом их зависимостей
// (Require) в плоский список без дубликатов. Зависимости идут после
// программы, которая их требует.
func expandPrograms(programs []Executable) []Executable {
	var result []Executable
	seen := make(map[string]bool)
	var walk func([]Executable)
	walk = func(list []Executable) {
		for _, p := range list {
			if seen[p.Name()] {
				continue
			}
			seen[p.Name()] = true
			result = append(result, p)
			walk(p.Require())
		}
	}
	walk(programs)
	return result
}

// Ensure гарантирует, что переданные программы и их зависимости доступны
// локально. Если нужной версии нет ни в системе, ни в папке инструментов,
// программа резолвится (определяется конкретный URL) и скачивается. Сеть
// используется только при отсутствии нужной версии.
func (m *Manager) Ensure(executables ...Executable) ([]Executable, error) {
	if err := os.MkdirAll(m.dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %v", m.dir, err)
	}
	flat := expandPrograms(executables)
	current := make(map[string]Executable)
	for _, p := range flat {
		resolved, err := m.resolve(p)
		if err != nil {
			return nil, err
		}
		if m.IsInstalled(resolved) {
			current[resolved.Name()] = resolved
			continue
		}
		fmt.Printf("Downloading %s %s\n", resolved.Name(), resolved.Version())
		if err := m.download(resolved); err != nil {
			return nil, err
		}
		current[resolved.Name()] = resolved
	}
	return rebuildWithDeps(flat, current), nil
}

// rebuildWithDeps собирает итоговый список программ, заменяя каждую программу
// на актуальную установленную версию и обновляя зависимости (Require):
// каждая зависимость заменяется на её актуальную установленную версию.
func rebuildWithDeps(flat []Executable, current map[string]Executable) []Executable {
	result := make([]Executable, 0, len(flat))
	for _, p := range flat {
		cp := current[p.Name()]
		var require []Executable
		for _, dep := range p.Require() {
			if act, ok := current[dep.Name()]; ok {
				require = append(require, act)
			} else {
				require = append(require, dep)
			}
		}
		if len(require) > 0 {
			cp = setRequire(cp, require)
		}
		result = append(result, cp)
	}
	return result
}

// setRequire возвращает копию программы ex с заданным списком зависимостей.
// Для рантаймов (без собственных зависимостей) список не меняется.
func setRequire(ex Executable, require []Executable) Executable {
	if pr, ok := ex.(*Program); ok {
		np := *pr
		np.require = require
		return &np
	}
	return ex
}

// resolve определяет версию и URL для программы. Для рантаймов (php/go)
// сначала выбирается уже доступный кандидат (системный или скачанный) без
// сети; если такого нет — определяются URL и полная версия через сеть.
// Для обычных программ (не рантаймов) резолюции не требуется.
func (m *Manager) resolve(ex Executable) (Executable, error) {
	rt, ok := ex.(Runtime)
	if !ok {
		return ex, nil
	}
	return m.resolveRuntime(rt)
}

// resolveRuntime выбирает рантайм: сначала подходящего кандидата без сети,
// затем, при отсутствии, определяет URL для скачивания.
func (m *Manager) resolveRuntime(rt Runtime) (Executable, error) {
	if cand, ok := rt.Resolve(m.dir, rt.Version()); ok {
		if sys, isRT := cand.(Runtime); isRT && sys.IsSystem() {
			fmt.Printf("Using local %s %s\n", cand.Name(), cand.Version())
		} else {
			fmt.Printf("Using downloaded %s %s\n", cand.Name(), cand.Version())
		}
		return cand, nil
	}
	// Сеть используется только когда реально нужно скачивание.
	return rt.ResolveDownload()
}
