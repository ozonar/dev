// Package selfconfig управляет общим пользовательским конфигом dev
// (~/dev-command/main.conf и системным /etc/dev-command/main.conf):
// путями, чтением параметров и редактированием.
package selfconfig

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dev/internal/common"
	"dev/internal/i18n"
)

const (
	// DirName — имя каталога конфигурации dev в домашней и системной директориях.
	DirName = "dev-command"

	// UserPath — пользовательский конфиг (высший приоритет).
	UserPath = "~/" + DirName + "/main.conf"
	// EtcPath — системный конфиг (используется, если пользовательского нет).
	EtcPath = "/etc/" + DirName + "/main.conf"
)

// Ключи параметров main.conf. Это единственное место, где заданы имена
// параметров; остальные пакеты ссылаются на эти константы.
const (
	KeyInterfaceLang = "INTERFACE_LANG"
	KeyLLMEndpoint   = "LLM_ENDPOINT"
	KeyLLMToken      = "LLM_TOKEN"
	KeyLLMModel      = "LLM_MODEL"
	KeyRecipeRepo    = "RECIPE_REPO"
)

// Param описывает параметр main.conf: ключ, комментарий для шаблона и
// значение по умолчанию.
type Param struct {
	Key     string
	Comment string
	Default string
}

// Params — единственный реестр параметров main.conf. Шаблон конфига
// строится из этого списка, поэтому перечень ключей не дублируется.
var Params = []Param{
	{KeyInterfaceLang, "Interface language: auto, en, ru", "auto"},
	{KeyLLMEndpoint, "OpenAI-compatible API endpoint (e.g. https://3qa.ru/api/v1/chat/completions)", ""},
	{KeyLLMToken, "API token for LLM access", ""},
	{KeyLLMModel, "Model name (e.g. deepseek/deepseek-v3.2)", ""},
	{KeyRecipeRepo, "Recipe repository: local path or HTTP(S) base URL containing repository.yml and the recipes/ directory. Leave empty to use the built-in default repository.", ""},
}

// DefaultTemplate возвращает содержимое main.conf, создаваемое при первом
// редактировании. Шаблон обязан быть валидным INI-подобным файлом:
// строки "ключ=значение".
func DefaultTemplate() string {
	var b strings.Builder
	b.WriteString("# dev command configuration\n")
	for _, p := range Params {
		b.WriteString("\n# " + p.Comment + "\n")
		b.WriteString(p.Key + "=" + p.Default + "\n")
	}
	return b.String()
}

// UserDir возвращает путь к пользовательскому каталогу конфигурации dev.
func UserDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DirName), nil
}

// EtcDir возвращает путь к системному каталогу конфигурации dev.
func EtcDir() string {
	return "/etc/" + DirName
}

// Paths возвращает пути к конфигу в порядке приоритета:
// 1. ~/dev-command/main.conf
// 2. /etc/dev-command/main.conf
func Paths() []string {
	return []string{
		common.ExpandHome(UserPath),
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
	return Value(KeyInterfaceLang)
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
		if err := os.WriteFile(path, []byte(DefaultTemplate()), 0644); err != nil {
			return fmt.Errorf(i18n.T("failed to create default config: %w"), err)
		}
		i18n.Yellow("Created default config at %s", path)
	}

	// Открываем в редакторе.
	editor := common.EditorCommand()
	i18n.Cyan("Opening config in %s...", editor[0])
	if err := common.OpenInEditor(path); err != nil {
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
