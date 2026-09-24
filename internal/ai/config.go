package ai

import (
	"bufio"
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

// LoadConfig загружает конфиг из файла.
// Ищет сначала ~/dev-config/main.conf, затем /etc/dev-command/main.conf.
func LoadConfig() (*Config, error) {
	var lastErr error
	for _, p := range ConfigPaths() {
		data, err := os.ReadFile(p)
		if err != nil {
			lastErr = err
			continue
		}

		cfg := &Config{}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			switch key {
			case "LLM_ENDPOINT":
				cfg.Endpoint = val
			case "LLM_TOKEN":
				cfg.Token = val
			case "LLM_MODEL":
				cfg.Model = val
			case "LANGUAGE":
				cfg.Language = val
			}
		}

		if cfg.Endpoint == "" || cfg.Token == "" || cfg.Model == "" {
			lastErr = fmt.Errorf(i18n.T("incomplete config at %s: need ENDPOINT, TOKEN, MODEL"), p)
			continue
		}

		return cfg, nil
	}

	return nil, fmt.Errorf(i18n.T("config file not found: %w"), lastErr)
}

// LanguageFromConfig возвращает язык интерфейса из конфигурации (ключ LANGUAGE).
// Читается из первого существующего конфига без строгой валидации остальных
// параметров, поэтому язык доступен даже при неполном конфиге. Пустая строка
// означает автоопределение из окружения.
func LanguageFromConfig() string {
	for _, p := range ConfigPaths() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			if strings.TrimSpace(parts[0]) == "LANGUAGE" {
				return strings.TrimSpace(parts[1])
			}
		}
		// Первый существующий конфиг определяет язык, даже если в нём нет
		// ключа LANGUAGE: файлы ниже по приоритету не рассматриваются.
		return ""
	}
	return ""
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
		return fmt.Errorf(i18n.T("config required"))
	}
	return EditConfig()
}
