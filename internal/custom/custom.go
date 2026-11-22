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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"dev/internal/common"
	"dev/internal/i18n"
	"dev/internal/selfconfig"

	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

// configPath — путь к файлу пользовательских команд (относительно home).
const configPath = "~/" + selfconfig.DirName + "/custom.yml"

// localFileName — имя файла локальных пользовательских команд в директории
// запуска команды dev.
const localFileName = ".custom"

// Context содержит параметры, пробрасываемые в пользовательскую команду:
// текущий путь, язык и фреймворк проекта, позиционные аргументы,
// дополнительные переменные окружения и источник ввода для параметров.
type Context struct {
	Dir       string            // текущий путь
	Language  string            // язык проекта
	Framework string            // фреймворк проекта
	Args      []string          // Args — позиционные аргументы, доступные в подкомандах как $1..$N.
	Env       map[string]string // Env — дополнительные переменные окружения
	Input     io.Reader         // Input — источник ввода для интерактивных параметров; nil — os.Stdin.
}

// Param описывает параметр команды. Значение берётся из аргумента NAME=value,
// затем из переменной окружения NAME; если значения нет, оно запрашивается
// интерактивно. Значение передаётся подкомандам как переменная окружения.
type Param struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Default     string `yaml:"default"`
}

// Command описывает одну пользовательскую команду — список подкоманд.
// Path ограничивает доступность команды: она выполняется только когда текущая
// директория находится внутри указанного пути (пустая строка — без ограничений).
type Command struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Subcommands []string `yaml:"subcommands"`
	// Params — параметры команды, запрашиваемые интерактивно при отсутствии
	// значения в аргументах и окружении.
	Params []Param `yaml:"params"`
	// Path — путь, внутри которого команда доступна. Абсолютный путь либо "~"
	// резолвятся как есть; относительный путь считается относительно текущей
	// директории запуска. Пустое значение — команда доступна везде.
	Path string `yaml:"path"`
}

// Config — корневая структура конфигурационного файла custom.yml.
type Config struct {
	Commands map[string]Command `yaml:"commands"`
}

// defaultConfigTemplate — содержимое custom.yml, создаваемое при первом
// редактировании. Шаблон обязан быть валидным YAML: в отступах допустимы
// только пробелы.
const defaultConfigTemplate = `# dev custom commands
#
# Available variables: $(current_dir), $(language), $(framework).
# The path field restricts a command: it runs only when the current directory
# is inside the given path (empty means no restriction).

commands:
  example:
    subcommands:
      - echo "Hello from $(current_dir) [$(language)/$(framework)]"
`

// ConfigFilePath возвращает абсолютный путь к файлу custom.yml.
func ConfigFilePath() string {
	return common.ExpandHome(configPath)
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
	for _, name := range cfg.Names() {
		if err := ValidateParams(cfg.Commands[name].Params); err != nil {
			return nil, fmt.Errorf("parse %s: %s: %w", path, name, err)
		}
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

	target := common.ExpandHome(c.Path)
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

	paramEnv, args, err := resolveParams(cmd.Params, ctx.Args, ctx.Input)
	if err != nil {
		return true, err
	}
	env := mergeEnv(ctx.Env, paramEnv)

	for _, raw := range cmd.Subcommands {
		line := Expand(raw, ctx)
		color.Cyan("> %s", line)

		// Аргументы передаются позиционно после текста скрипта: в нём они
		// доступны как $1..$N и не участвуют в разборе shell-синтаксиса.
		argv := append([]string{"-c", line, name}, args...)
		sh := exec.Command("sh", argv...)
		sh.Dir = ctx.Dir
		sh.Env = commandEnv(env)
		sh.Stdin = os.Stdin
		sh.Stdout = os.Stdout
		sh.Stderr = os.Stderr
		if err := sh.Run(); err != nil {
			return true, fmt.Errorf(i18n.T("command %q failed: %w"), name, err)
		}
	}
	return true, nil
}

// ValidateParams проверяет, что имена параметров пригодны для использования
// как имена переменных окружения и не повторяются.
func ValidateParams(params []Param) error {
	seen := make(map[string]struct{}, len(params))
	for _, param := range params {
		if !isEnvName(param.Name) {
			return fmt.Errorf(i18n.T("invalid parameter name %q"), param.Name)
		}
		if _, ok := seen[param.Name]; ok {
			return fmt.Errorf(i18n.T("duplicate parameter %q"), param.Name)
		}
		seen[param.Name] = struct{}{}
	}
	return nil
}

// isEnvName проверяет, что строка является допустимым именем переменной
// окружения: латинская буква или "_", далее буквы, цифры или "_".
func isEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r == '_', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// splitArgs отделяет аргументы NAME=value с объявленными именами параметров
// от позиционных аргументов. Неизвестные NAME=value остаются позиционными.
func splitArgs(params []Param, args []string) (map[string]string, []string) {
	declared := make(map[string]struct{}, len(params))
	for _, param := range params {
		declared[param.Name] = struct{}{}
	}
	values := make(map[string]string)
	positional := make([]string, 0, len(args))
	for _, arg := range args {
		name, value, ok := strings.Cut(arg, "=")
		if ok {
			if _, known := declared[name]; known {
				values[name] = value
				continue
			}
		}
		positional = append(positional, arg)
	}
	return values, positional
}

// resolveParams определяет значения параметров команды. Приоритет:
// аргумент NAME=value, затем переменная окружения, затем интерактивный ввод,
// затем значение по умолчанию. Возвращает окружение подкоманд и позиционные
// аргументы $1..$N.
func resolveParams(params []Param, args []string, in io.Reader) (map[string]string, []string, error) {
	values, positional := splitArgs(params, args)
	for _, param := range params {
		if _, ok := values[param.Name]; ok {
			continue
		}
		if value, ok := os.LookupEnv(param.Name); ok {
			values[param.Name] = value
			continue
		}
		value, err := promptParam(param, in)
		if err != nil {
			return nil, nil, err
		}
		values[param.Name] = value
	}
	return values, positional, nil
}

// promptParam запрашивает значение параметра. Пустой ввод принимает значение
// по умолчанию; параметр без значения по умолчанию обязателен.
func promptParam(param Param, in io.Reader) (string, error) {
	if in == nil {
		in = os.Stdin
	}
	prompt := param.Name
	if param.Description != "" {
		prompt += " — " + param.Description
	}
	if param.Default != "" {
		prompt += " [" + param.Default + "]"
	}
	for {
		i18n.Printf("%s: ", prompt)
		line, ok := common.ReadLine(in)
		if !ok {
			return "", fmt.Errorf(i18n.T("parameter %q is required but no value was provided"), param.Name)
		}
		value := strings.TrimSpace(line)
		if value != "" {
			return value, nil
		}
		if param.Default != "" {
			return param.Default, nil
		}
		i18n.Yellow("Value is required.")
	}
}

// mergeEnv объединяет базовое окружение с окружением параметров: значения
// параметров приоритетнее.
func mergeEnv(base, extra map[string]string) map[string]string {
	if len(base) == 0 {
		return extra
	}
	merged := make(map[string]string, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}

// commandEnv возвращает окружение процесса с добавленными переменными extra.
// Ключи сортируются для детерминированного порядка; при совпадении имён
// побеждает значение из extra
func commandEnv(extra map[string]string) []string {
	env := os.Environ()
	if len(extra) == 0 {
		return env
	}
	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+extra[key])
	}
	return env
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
		return fmt.Errorf(i18n.T("failed to create config directory: %w"), err)
	}

	// Создаём файл с шаблоном, если он не существует.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(defaultConfigTemplate), 0644); err != nil {
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
