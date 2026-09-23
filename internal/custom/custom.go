// Package custom реализует пользовательские команды, хранящиеся в
// глобальном конфигурационном файле ~/dev-command/custom.yml и в локальном
// файле .custom директории запуска команды dev (локальные команды имеют
// приоритет при совпадении имён).
//
// Незнакомая команда dev <name> сверяется со списком команд из конфигов и,
// если найдена, последовательно выполняет её подкоманды с подстановкой
// переменных $(current_dir), $(language), $(framework). При падении любой
// подкоманды выполнение останавливается.
//
// Каждая команда может быть ограничена путём через поле path: она выполняется
// только когда текущая директория находится внутри указанного пути.
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

// localFileName — имя файла локальных пользовательских команд в директории
// запуска команды dev.
const localFileName = ".custom"

// Context содержит параметры, пробрасываемые в пользовательскую команду:
// текущий путь, язык и фреймворк проекта.
type Context struct {
	Dir       string // текущий путь
	Language  string // язык проекта
	Framework string // фреймворк проекта
}

// Command описывает одну пользовательскую команду — список подкоманд.
// Path ограничивает доступность команды: она выполняется только когда текущая
// директория находится внутри указанного пути (пустая строка — без ограничений).
type Command struct {
	Subcommands []string `yaml:"subcommands"`
	// Path — путь, внутри которого команда доступна. Абсолютный путь либо "~"
	// резолвятся как есть; относительный путь считается относительно текущей
	// директории запуска. Пустое значение — команда доступна везде.
	Path string `yaml:"path"`
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

// Load читает глобальный конфигурационный файл custom.yml (~/dev-command).
// Если файла нет — возвращает пустой конфиг без ошибки.
func Load() (*Config, error) {
	return loadFile(ConfigFilePath())
}

// LoadLocal читает локальный файл кастомных команд (.custom) из директории
// запуска команды dev. Если файла нет — возвращает пустой конфиг без ошибки.
func LoadLocal(cwd string) (*Config, error) {
	return loadFile(filepath.Join(cwd, localFileName))
}

// loadFile читает и разбирает YAML-конфиг кастомных команд из указанного файла.
// Отсутствие файла не является ошибкой — возвращается пустой конфиг.
func loadFile(path string) (*Config, error) {
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

// LoadAll загружает глобальный конфиг и локальный .custom из директории
// запуска, объединяя их. При совпадении имён команда из локального файла
// имеет приоритет — она специфична для конкретного проекта.
func LoadAll(cwd string) (*Config, error) {
	global, err := Load()
	if err != nil {
		return nil, err
	}
	local, err := LoadLocal(cwd)
	if err != nil {
		return nil, err
	}
	for name, cmd := range local.Commands {
		global.Commands[name] = cmd
	}
	return global, nil
}

// LocalFilePath возвращает путь к локальному файлу кастомных команд (.custom)
// внутри указанной директории запуска.
func LocalFilePath(cwd string) string {
	return filepath.Join(cwd, localFileName)
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

// NamesFor возвращает отсортированный список имён команд, доступных в заданном
// контексте (с учётом ограничений по пути).
func (c *Config) NamesFor(ctx Context) []string {
	names := make([]string, 0, len(c.Commands))
	for n, cmd := range c.Commands {
		if !cmd.allowedByPath(ctx.Dir) {
			continue
		}
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

// allowedByPath проверяет, проходит ли команда ограничение по пути Path.
// Пустое значение Path означает доступность в любом месте. Относительные пути
// интерпретируются относительно текущей директории запуска (dir).
func (c Command) allowedByPath(dir string) bool {
	if c.Path == "" {
		return true
	}

	target := resolvePath(c.Path)
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(absTarget, absDir)
	if err != nil {
		return false
	}
	// Текущая директория внутри целевой, если относительный путь не выходит
	// за пределы цели (".." либо ".."+разделитель).
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// RunCommand выполняет пользовательскую команду по имени.
// Возвращает (false, nil), если команда с таким именем не найдена либо её
// ограничение по пути не допускает запуск из текущей директории.
// Подкоманды выполняются последовательно; при падении любой из них
// выполнение прерывается и возвращается ошибка.
func (c *Config) RunCommand(name string, ctx Context) (bool, error) {
	cmd, ok := c.Commands[name]
	if !ok || !cmd.allowedByPath(ctx.Dir) {
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
#
# Глобальные команды хранятся здесь (~/dev-command/custom.yml).
# Локальные команды проекта можно положить в файл .custom в корне проекта —
# они будут доступны только при запуске dev из этого каталога и переопределяют
# глобальные команды с теми же именами.
#
# Доступные переменные: $(current_dir), $(language), $(framework).
# Поле path ограничивает команду: она выполняется только когда текущая
# директория находится внутри указанного пути (пусто — без ограничений).

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
