package ai

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
	UserConfigPath = "~/dev-config/main.conf"
	EtcConfigPath  = "/etc/dev-command/main.conf"
)

// Config хранит параметры подключения к LLM и язык интерфейса.
type Config struct {
	Endpoint string
	Token    string
	Model    string
	// Language — язык вывода приложения (ключ LANGUAGE в конфиге).
	// Пустое значение означает автоопределение из окружения (LC_ALL/LANG).
	Language string
}

// resolvePath заменяет ~ на home директорию
func resolvePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// ConfigPaths возвращает пути к конфигу в порядке приоритета:
// 1. ~/dev-config/main.conf
// 2. /etc/dev-command/main.conf
func ConfigPaths() []string {
	return []string{
		resolvePath(UserConfigPath),
		EtcConfigPath,
	}
}

// resolveConfigPath находит первый существующий конфиг или возвращает
// путь с наивысшим приоритетом, если ни одного нет.
func resolveConfigPath() string {
	for _, p := range ConfigPaths() {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// Если ни одного нет — возвращаем пользовательский путь
	return ConfigPaths()[0]
}

// configParam связывает ключ конфига с полем структуры Config.
type configParam struct {
	key   string
	field func(*Config) *string
}

// configParams — единственное место, где перечислены параметры конфига.
// По этой таблице и читается файл, и накладывается окружение, поэтому
// отдельного списка переменных окружения не существует.
var configParams = []configParam{
	{"LLM_ENDPOINT", func(c *Config) *string { return &c.Endpoint }},
	{"LLM_TOKEN", func(c *Config) *string { return &c.Token }},
	{"LLM_MODEL", func(c *Config) *string { return &c.Model }},
	{"LANGUAGE", func(c *Config) *string { return &c.Language }},
}

// readConfigFile читает файл конфига в набор "ключ → значение".
// Имена ключей здесь не проверяются: отбор известных параметров выполняет
// resolveValues по таблице configParams.
func readConfigFile(path string) (map[string]string, error) {
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

// resolveValues возвращает итоговые значения параметров: переменная окружения
// с именем ключа приоритетнее значения из файла. Правило одно для всех
// параметров, поэтому источник каждого значения предсказуем.
func resolveValues(fileValues map[string]string) map[string]string {
	values := make(map[string]string, len(configParams))
	for _, p := range configParams {
		if v := os.Getenv(p.key); v != "" {
			values[p.key] = v
			continue
		}
		if v, ok := fileValues[p.key]; ok {
			values[p.key] = v
		}
	}
	return values
}

// applyValues переносит разрешённые значения в поля структуры.
func (c *Config) applyValues(values map[string]string) {
	for _, p := range configParams {
		if v, ok := values[p.key]; ok {
			*p.field(c) = v
		}
	}
}

// complete сообщает, что заданы все обязательные параметры подключения.
func (c *Config) complete() bool {
	return c.Endpoint != "" && c.Token != "" && c.Model != ""
}

// LoadConfig загружает параметры конфига. Значения берутся из файла
// (~/dev-config/main.conf, затем /etc/dev-command/main.conf) и окружения:
// переменная с именем ключа приоритетнее файла. Если файла нет, параметры
// могут быть полностью заданы окружением (CI).
func LoadConfig() (*Config, error) {
	var lastErr error
	for _, p := range ConfigPaths() {
		fileValues, err := readConfigFile(p)
		if err != nil {
			lastErr = err
			continue
		}

		cfg := &Config{}
		cfg.applyValues(resolveValues(fileValues))
		if !cfg.complete() {
			lastErr = fmt.Errorf(i18n.T("incomplete config at %s: need ENDPOINT, TOKEN, MODEL"), p)
			continue
		}
		return cfg, nil
	}

	// Файла нет: параметры могут быть заданы только окружением.
	cfg := &Config{}
	cfg.applyValues(resolveValues(nil))
	if cfg.complete() {
		return cfg, nil
	}

	return nil, fmt.Errorf(i18n.T("config file not found: %w"), lastErr)
}

// LanguageFromConfig возвращает язык интерфейса: LANGUAGE из окружения
// или из первого существующего конфига. Пустая строка означает
// автоопределение из окружения (LC_ALL/LANG).
func LanguageFromConfig() string {
	var fileValues map[string]string
	for _, p := range ConfigPaths() {
		values, err := readConfigFile(p)
		if err != nil {
			continue
		}
		fileValues = values
		break
	}
	return resolveValues(fileValues)["LANGUAGE"]
}

// EditConfig открывает конфиг на редактирование, создавая при необходимости.
// Создаёт/редактирует ~/dev-config/main.conf (пользовательский путь).
func EditConfig() error {
	path := ConfigPaths()[0]

	// Создаём папку если нет
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf(i18n.T("failed to create config directory: %w"), err)
	}

	// Создаём файл с пустыми параметрами если нет
	if _, err := os.Stat(path); os.IsNotExist(err) {
		defaultCfg := `# dev command configuration

# Output language: auto (from LC_ALL/LANG), en, ru
LANGUAGE=auto

# OpenAI-compatible API endpoint (e.g. https://3qa.ru/api/v1/chat/completions)
LLM_ENDPOINT=

# API token for LLM access
LLM_TOKEN=

# Model name (e.g. deepseek/deepseek-v3.2)
LLM_MODEL=
`
		if err := os.WriteFile(path, []byte(defaultCfg), 0644); err != nil {
			return fmt.Errorf(i18n.T("failed to create default config: %w"), err)
		}
		i18n.Yellow("Created default config at %s", path)
	}

	// Открываем в редакторе
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

// InteractiveEditConfig спрашивает пользователя и открывает редактор
func InteractiveEditConfig() error {
	reader := bufio.NewReader(os.Stdin)
	i18n.Yellow("Config file not found or incomplete at %s", resolveConfigPath())
	i18n.Printf("Open config in editor? [Y/n]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "n" || input == "no" {
		return errors.New(i18n.T("config required"))
	}
	return EditConfig()
}
