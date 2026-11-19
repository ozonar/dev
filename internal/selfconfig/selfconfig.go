// Package selfconfig управляет общим пользовательским конфигом dev
// (~/dev-command/main.conf и системным /etc/dev-command/main.conf):
// путями, чтением параметров и редактированием.
package selfconfig

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dev/internal/i18n"
)

const (
	// UserPath — пользовательский конфиг (высший приоритет).
	UserPath = "~/dev-command/main.conf"
	// EtcPath — системный конфиг (используется, если пользовательского нет).
	EtcPath = "/etc/dev-command/main.conf"
)

// defaultTemplate — содержимое main.conf, создаваемое при первом редактировании.
// Шаблон обязан быть валидным INI-подобным файлом: строки "ключ=значение".
const defaultTemplate = `# dev command configuration

# Interface language: auto, en, ru
INTERFACE_LANG=auto

# OpenAI-compatible API endpoint (e.g. https://3qa.ru/api/v1/chat/completions)
LLM_ENDPOINT=

# API token for LLM access
LLM_TOKEN=

# Model name (e.g. deepseek/deepseek-v3.2)
LLM_MODEL=

# Recipe repository: local path or HTTP(S) base URL containing
# repository.yml and the recipes/ directory.
# Default: https://raw.githubusercontent.com/ozonar/dev-recipe/main
RECIPE_REPO=
`

// resolvePath заменяет "~" и префикс "~/" на домашнюю директорию пользователя.
func resolvePath(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// Paths возвращает пути к конфигу в порядке приоритета:
// 1. ~/dev-command/main.conf
// 2. /etc/dev-command/main.conf
func Paths() []string {
	return []string{
		resolvePath(UserPath),
		EtcPath,
	}
}

// ReadFile читает файл конфига в набор "ключ → значение".
// Строки без "=" и комментарии игнорируются.
func ReadFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return values, nil
}

// ApplyEnv возвращает значения конфига, в которых переменная окружения с
// именем ключа приоритетнее значения из файла. Окружение добавляется целиком,
// чтобы параметры можно было задать без файла (например, в CI); отбор нужных
// ключей выполняет вызывающая сторона по своей таблице параметров.
func ApplyEnv(fileValues map[string]string) map[string]string {
	values := make(map[string]string, len(fileValues))
	for key, value := range fileValues {
		values[key] = value
	}
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || parts[1] == "" {
			continue
		}
		values[parts[0]] = parts[1]
	}
	return values
}

// normalizeKey приводит имя параметра к нижнему регистру.
func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// Value возвращает значение параметра: переменная окружения приоритетнее
// файла, файлы проверяются в порядке приоритета. Имена ключей в файле
// сравниваются без учёта регистра. Если параметра нет — возвращается
// пустая строка.
func Value(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	want := normalizeKey(key)
	for _, path := range Paths() {
		values, err := ReadFile(path)
		if err != nil {
			continue
		}
		for k, v := range values {
			if normalizeKey(k) == want {
				return v
			}
		}
	}
	return ""
}

// Language возвращает язык интерфейса из конфига.
func Language() string {
	return Value("INTERFACE_LANG")
}

// resolveConfigPath находит первый существующий конфиг или возвращает
// путь с наивысшим приоритетом, если ни одного нет.
func resolveConfigPath() string {
	for _, path := range Paths() {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return Paths()[0]
}

// Dir возвращает директорию активного конфига.
func Dir() string {
	return filepath.Dir(resolveConfigPath())
}

// Edit открывает конфиг на редактирование, создавая при необходимости
// файл с шаблоном. Если существует только системный конфиг, редактируется
// он; иначе создаётся пользовательский. Использует $EDITOR или nano.
func Edit() error {
	path := resolveConfigPath()

	// Создаём папку конфигурации, если её нет.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf(i18n.T("failed to create config directory: %w"), err)
	}

	// Создаём файл с шаблоном, если он не существует.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(defaultTemplate), 0644); err != nil {
			return fmt.Errorf(i18n.T("failed to create default config: %w"), err)
		}
		i18n.Yellow("Created default config at %s", path)
	}

	// Открываем в редакторе.
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}

	i18n.Cyan("Opening config in %s...", editor)
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(i18n.T("editor failed: %w"), err)
	}

	i18n.Green("Config saved.")
	return nil
}

// InteractiveEdit спрашивает пользователя и открывает редактор.
func InteractiveEdit() error {
	reader := bufio.NewReader(os.Stdin)
	i18n.Yellow("Config file not found or incomplete at %s", resolveConfigPath())
	i18n.Printf("Open config in editor? [Y/n]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "n" || input == "no" {
		return errors.New(i18n.T("config required"))
	}
	return Edit()
}
