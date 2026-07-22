package ai

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

const configPath = "/etc/dev-command/main.conf"

// Config хранит параметры подключения к LLM
type Config struct {
	Endpoint string
	Token    string
	Model    string
}

// LoadConfig загружает конфиг из файла
func LoadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("config file not found: %w", err)
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
		}
	}

	if cfg.Endpoint == "" || cfg.Token == "" || cfg.Model == "" {
		return nil, fmt.Errorf("incomplete config: need ENDPOINT, TOKEN, MODEL")
	}

	return cfg, nil
}

// EditConfig открывает конфиг на редактирование, создавая при необходимости
func EditConfig() error {
	// Создаём папку если нет
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Создаём файл с пустыми параметрами если нет
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultCfg := `# dev command configuration

# OpenAI-compatible API endpoint (e.g. https://3qa.ru/api/v1/chat/completions)
LLM_ENDPOINT=

# API token for LLM access
LLM_TOKEN=

# Model name (e.g. deepseek/deepseek-v3.2)
LLM_MODEL=
`
		if err := os.WriteFile(configPath, []byte(defaultCfg), 0644); err != nil {
			return fmt.Errorf("failed to create default config: %w", err)
		}
		color.Yellow("Created default config at %s", configPath)
	}

	// Открываем в редакторе
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}

	color.Cyan("Opening config in %s...", editor)
	cmd := exec.Command(editor, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor failed: %w", err)
	}

	color.Green("Config saved.")
	return nil
}

// InteractiveEditConfig спрашивает пользователя и открывает редактор
func InteractiveEditConfig() error {
	reader := bufio.NewReader(os.Stdin)
	color.Yellow("Config file not found or incomplete at %s", configPath)
	fmt.Print("Open config in editor? [Y/n]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "n" || input == "no" {
		return fmt.Errorf("config required")
	}
	return EditConfig()
}
