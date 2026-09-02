// Package custom реализует пользовательские команды, хранящиеся в
// конфигурационном файле ~/dev-command/custom.yml.
//
// Незнакомая команда dev <name> сверяется со списком команд из конфига и,
// если найдена, последовательно выполняет её подкоманды с подстановкой
// переменных $(current_dir), $(language), $(framework). При падении любой
// подкоманды выполнение останавливается.
package custom

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

// configPath — путь к файлу пользовательских команд (относительно home).
const configPath = "~/dev-command/custom.yml"

// Context содержит параметры, пробрасываемые в пользовательскую команду:
// текущий путь, язык и фреймворк проекта.
type Context struct {
	Dir       string // текущий путь
	Language  string // язык проекта
	Framework string // фреймворк проекта
}

// Command описывает одну пользовательскую команду — список подкоманд.
type Command struct {
	Subcommands []string `yaml:"subcommands"`
}

// Config — корневая структура конфигурационного файла custom.yml.
type Config struct {
	Commands map[string]Command `yaml:"commands"`
}

// resolvePath заменяет префикс "~/" на домашнюю директорию пользователя.
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

// ConfigFilePath возвращает абсолютный путь к файлу custom.yml.
func ConfigFilePath() string {
	return resolvePath(configPath)
}

// Load читает конфигурационный файл custom.yml.
// Если файла нет — возвращает пустой конфиг без ошибки.
func Load() (*Config, error) {
	path := ConfigFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Commands: map[string]Command{}}, nil
		}
		return nil, err
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Commands == nil {
		cfg.Commands = map[string]Command{}
	}
	return cfg, nil
}

// Names возвращает отсортированный список имён пользовательских команд.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Commands))
	for n := range c.Commands {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Has проверяет наличие команды с указанным именем.
func (c *Config) Has(name string) bool {
	_, ok := c.Commands[name]
	return ok
}

// RunCommand выполняет пользовательскую команду по имени.
// Возвращает (false, nil), если команда с таким именем не найдена.
// Подкоманды выполняются последовательно; при падении любой из них
// выполнение прерывается и возвращается ошибка.
func (c *Config) RunCommand(name string, ctx Context) (bool, error) {
	cmd, ok := c.Commands[name]
	if !ok {
		return false, nil
	}

	for _, raw := range cmd.Subcommands {
		line := Expand(raw, ctx)
		color.Cyan("> %s", line)

		sh := exec.Command("sh", "-c", line)
		sh.Dir = ctx.Dir
		sh.Stdin = os.Stdin
		sh.Stdout = os.Stdout
		sh.Stderr = os.Stderr
		if err := sh.Run(); err != nil {
			return true, fmt.Errorf("command %q failed: %w", name, err)
		}
	}
	return true, nil
}

// Expand подставляет в строку команды переменные окружения:
// $(current_dir), $(language), $(framework).
func Expand(line string, ctx Context) string {
	r := strings.NewReplacer(
		"$(current_dir)", ctx.Dir,
		"$(language)", ctx.Language,
		"$(framework)", ctx.Framework,
	)
	return r.Replace(line)
}

// Edit открывает custom.yml на редактирование, создавая файл с шаблоном,
// если он ещё не существует. Использует $EDITOR или nano по умолчанию.
func Edit() error {
	path := ConfigFilePath()

	// Создаём папку конфигурации, если её нет.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Создаём файл с шаблоном, если он не существует.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		defaultCfg := `# dev custom commands

commands:
  example:
    subcommands:
      - echo "Hello from $(current_dir) [$(language)/$(framework)]"
`
		if err := os.WriteFile(path, []byte(defaultCfg), 0644); err != nil {
			return fmt.Errorf("failed to create default config: %w", err)
		}
		color.Yellow("Created default config at %s", path)
	}

	// Открываем в редакторе.
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}

	color.Cyan("Opening config in %s...", editor)
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor failed: %w", err)
	}

	color.Green("Config saved.")
	return nil
}
