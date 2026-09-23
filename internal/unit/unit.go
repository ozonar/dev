// Package unit реализует запуск юнит-тестов проекта для поддерживаемых языков.
//
// Язык и фреймворк берутся из детектора проекта. Для каждого языка выбирается
// подходящий тестовый раннер:
//   - Go:        go test ./...
//   - PHP:       vendor/bin/phpunit (или composer-скрипт "test")
//   - JavaScript: npm|yarn|pnpm test
//   - Python:    pytest / manage.py test / unittest discover
//   - Ruby:      bin/rails test / bundle exec rspec / bundle exec rake test
//
// Дополнительные аргументы, переданные в dev unit, пробрасываются в выбранный
// раннер, что позволяет уточнять объём тестов (например go test ./internal/...).
package unit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dev/internal/toolchain"
)

// Options содержит параметры запуска юнит-тестов.
type Options struct {
	// Framework — фреймворк проекта (из детектора), например "django", "rails".
	Framework string
	// Language — язык проекта (php, go, javascript, python, ruby).
	Language string
	// Version — требуемая версия языка (например "8.3" для PHP).
	Version string
	// Args — дополнительные аргументы, передаваемые тестовому раннеру.
	Args []string
}

// Run запускает юнит-тесты проекта в зависимости от языка.
// Возвращает ошибку, если язык не поддерживается либо выполнение раннера
// завершилось неуспешно.
func Run(opts Options) error {
	switch opts.Language {
	case "go":
		return runGo(opts.Version, opts.Args)
	case "php":
		return runPHP(opts.Version, opts.Args)
	case "javascript":
		return runJS(opts.Args)
	case "python":
		return runPython(opts.Framework, opts.Args)
	case "ruby":
		return runRuby(opts.Framework, opts.Args)
	default:
		return fmt.Errorf("unsupported language for unit tests: %q", opts.Language)
	}
}

// runGo запускает go test. Без явных аргументов проверяются все пакеты (./...).
func runGo(version string, args []string) error {
	runtimePath, err := toolchain.ResolveRuntime("go", version)
	if err != nil {
		return err
	}
	cmd := exec.Command(runtimePath, buildGoArgs(args)...)
	// GOTOOLCHAIN=local запрещает go автоматически скачивать другую версию
	// тулчейна: используем ровно тот рантайм, который выбрал тулчейн-менеджер.
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// buildGoArgs собирает аргументы для go test: по умолчанию проверяются все
// пакеты (./...), при явных аргументах они передаются как есть.
func buildGoArgs(args []string) []string {
	if len(args) == 0 {
		return []string{"test", "./..."}
	}
	return append([]string{"test"}, args...)
}

// phpUnitCandidates — стандартные пути к исполняемому файлу PHPUnit
// в Composer-проектах.
var phpUnitCandidates = []string{
	"vendor/bin/phpunit",
	"bin/phpunit",
}

// runPHP запускает PHPUnit (через выбранный php-рантайм) либо, при его
// отсутствии, composer-скрипт "test".
func runPHP(version string, args []string) error {
	runtimePath, err := toolchain.ResolveRuntime("php", version)
	if err != nil {
		return err
	}

	// Приоритет — PHPUnit из зависимостей проекта.
	for _, candidate := range phpUnitCandidates {
		if fileExists(candidate) {
			cmdArgs := append([]string{candidate}, args...)
			return run(runtimePath, cmdArgs)
		}
	}

	// Fallback — composer-скрипт "test", если он объявлен в проекте.
	if hasComposerTestScript(".") {
		cmdArgs := append([]string{"test"}, args...)
		return run("composer", cmdArgs)
	}

	return fmt.Errorf("no PHP test runner found (looked for phpunit and composer \"test\" script)")
}

// runJS запускает тестовый скрипт package.json через пакетный менеджер проекта.
func runJS(args []string) error {
	pm := detectPackageManager(".")
	if pm == "" {
		return fmt.Errorf("no package manager found (npm/yarn/pnpm)")
	}
	if !hasNpmTestScript(".") {
		return fmt.Errorf("no \"test\" script in package.json")
	}
	cmdArgs := append([]string{"test"}, args...)
	return run(pm, cmdArgs)
}

// runPython запускает тесты Python-проекта: pytest (если он настроен или
// установлен), иначе manage.py test для Django, иначе unittest discover.
func runPython(framework string, args []string) error {
	runtimePath, err := toolchain.ResolveRuntime("python", "")
	if err != nil {
		return err
	}

	switch {
	case hasPytestConfig("."):
		cmdArgs := append([]string{"-m", "pytest"}, args...)
		return run(runtimePath, cmdArgs)
	case framework == "django" && fileExists("manage.py"):
		cmdArgs := append([]string{"manage.py", "test"}, args...)
		return run(runtimePath, cmdArgs)
	case hasPytestModule(runtimePath):
		cmdArgs := append([]string{"-m", "pytest"}, args...)
		return run(runtimePath, cmdArgs)
	default:
		cmdArgs := append([]string{"-m", "unittest", "discover"}, args...)
		return run(runtimePath, cmdArgs)
	}
}

// runRuby запускает тесты Ruby-проекта: rails test, rspec или rake test.
func runRuby(framework string, args []string) error {
	switch {
	case framework == "rails" && fileExists("bin/rails"):
		cmdArgs := append([]string{"test"}, args...)
		return run("bin/rails", cmdArgs)
	case dirExists("spec"):
		cmdArgs := append([]string{"exec", "rspec"}, args...)
		return run("bundle", cmdArgs)
	case fileExists("Rakefile") || fileExists("rakefile"):
		cmdArgs := append([]string{"exec", "rake", "test"}, args...)
		return run("bundle", cmdArgs)
	default:
		return fmt.Errorf("no Ruby test runner found (looked for rails test, rspec, rake test)")
	}
}

// detectPackageManager определяет пакетный менеджер Node.js по lock-файлам
// проекта: pnpm > yarn > npm. Возвращает пустую строку, если package.json
// отсутствует вовсе.
func detectPackageManager(dir string) string {
	if fileExists(filepath.Join(dir, "pnpm-lock.yaml")) ||
		fileExists(filepath.Join(dir, "pnpm-workspace.yaml")) {
		return "pnpm"
	}
	if fileExists(filepath.Join(dir, "yarn.lock")) {
		return "yarn"
	}
	if fileExists(filepath.Join(dir, "package-lock.json")) ||
		fileExists(filepath.Join(dir, "package.json")) {
		return "npm"
	}
	return ""
}

// hasNpmTestScript проверяет, что в package.json объявлен скрипт "test".
func hasNpmTestScript(dir string) bool {
	var cfg struct {
		Scripts map[string]interface{} `json:"scripts"`
	}
	if err := readJSON(filepath.Join(dir, "package.json"), &cfg); err != nil {
		return false
	}
	_, ok := cfg.Scripts["test"]
	return ok
}

// hasComposerTestScript проверяет, что в composer.json объявлен скрипт "test".
func hasComposerTestScript(dir string) bool {
	var cfg struct {
		Scripts map[string]interface{} `json:"scripts"`
	}
	if err := readJSON(filepath.Join(dir, "composer.json"), &cfg); err != nil {
		return false
	}
	_, ok := cfg.Scripts["test"]
	return ok
}

// hasPytestConfig проверяет наличие конфигурации pytest в стандартных файлах
// проекта (pytest.ini, tox.ini, pyproject.toml, setup.cfg).
func hasPytestConfig(dir string) bool {
	if fileExists(filepath.Join(dir, "pytest.ini")) {
		return true
	}
	for _, name := range []string{"tox.ini", "pyproject.toml", "setup.cfg"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		// setup.cfg использует секцию [tool:pytest], остальные — [pytest]
		// или [tool.pytest.ini_options].
		if strings.Contains(string(data), "[pytest]") ||
			strings.Contains(string(data), "[tool:pytest]") ||
			strings.Contains(string(data), "[tool.pytest.ini_options]") {
			return true
		}
	}
	return false
}

// hasPytestModule проверяет, что модуль pytest установлен в выбранном
// python-рантайме (без вывода на консоль).
func hasPytestModule(runtimePath string) bool {
	cmd := exec.Command(runtimePath, "-c", "import pytest")
	return cmd.Run() == nil
}

// readJSON читает и разбирает JSON-файл.
func readJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// fileExists проверяет, что по пути существует обычный файл.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists проверяет, что по пути существует директория.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// run запускает команду с потоковым выводом stdout/stderr в консоль.
func run(name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
