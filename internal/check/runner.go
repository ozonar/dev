package check

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dev/internal/i18n"
	"dev/internal/toolchain"
)

// Mode определяет режим запуска проверки.
type Mode int

const (
	// ModeDryRun — только показать проблемы (по умолчанию).
	ModeDryRun Mode = iota
	// ModeFix — автоматически исправить ошибки, где это возможно.
	ModeFix
)

// buildArgs формирует аргументы запуска для конкретной программы
// с учётом объёма проверки и режима (dry-run/fix).
// Второе возвращаемое значение ok=false означает, что в выбранном объёме
// нет файлов, подходящих этой программе, — запускать её не следует.
func buildArgs(prog toolchain.Executable, scope Scope, mode Mode) ([]string, bool) {
	var args []string
	switch prog.Name() {
	case "golangci-lint":
		// golangci-lint run [--fix] <dirs>
		// golangci-lint не принимает файлы из разных директорий одновременно
		// поэтому передаём уникальные директории изменённых файлов.
		// Директории без Go-файлов (например корневой "." при изменении README/go.mod)
		// отфильтровываем: golangci-lint падает с "no go files to analyze".
		args = append(args, "run")
		if mode == ModeFix {
			args = append(args, "--fix")
		}
		dirs := goDirArgs(scope.Dirs)
		if !appendPaths(&args, dirs, scope, "./...") {
			return nil, false
		}
	case "phpstan":
		// PHPStan не поддерживает автоисправление, поэтому режим fix не влияет.
		args = append(args, "analyse", "--memory-limit=1G", "--level=5")
		if !appendPaths(&args, scope.FilesWithExt(".php"), scope, ".") {
			return nil, false
		}
	case "php-cs-fixer":
		// php-cs-fixer fix [--dry-run] [--using-cache=no] [--config=...] <paths>.
		args = append(args, "fix")
		// Отключаем кэш, чтобы не создавать файл .php-cs-fixer.cache в проекте.
		args = append(args, "--using-cache=no")
		if mode == ModeDryRun {
			args = append(args, "--dry-run")
		}
		if cfg := ensurePhpCsFixerConfig("."); cfg != "" {
			args = append(args, "--config="+cfg)
		}
		if !appendPaths(&args, scope.FilesWithExt(".php"), scope, ".") {
			return nil, false
		}
	case "biome":
		// biome check [--write] <paths>. Одна команда объединяет lint, format
		// и импорт-сортировку. В dry-run без --write (только проверка),
		// в fix — с --write (применение исправлений).
		args = append(args, "check")
		if mode == ModeFix {
			args = append(args, "--write")
		}
		if !appendPaths(&args, scope.FilesWithExt(codeExtensionsFor("javascript")...), scope, ".") {
			return nil, false
		}
	case "ruff":
		// ruff check [--fix] <paths>. Линтинг и импорт-сортировка.
		// В dry-run без --fix (только проверка), в fix — с --fix.
		args = append(args, "check")
		if mode == ModeFix {
			args = append(args, "--fix")
		}
		if !appendPaths(&args, scope.FilesWithExt(".py"), scope, ".") {
			return nil, false
		}
	default:
		// Неизвестная программа — передаём пути как есть.
		args = append(args, scope.Files...)
	}
	return args, true
}

// appendPaths добавляет в args целевые пути проверки: конкретные файлы,
// если они есть, либо маркер всего кода (all) для полного объёма scopeAll.
// Возвращает false, когда целей нет вовсе — программу запускать не нужно
// (например, в fix-режиме изменились только файлы других типов).
func appendPaths(args *[]string, files []string, scope Scope, all string) bool {
	if len(files) > 0 {
		*args = append(*args, files...)
		return true
	}
	if scope.kind == scopeAll {
		*args = append(*args, all)
		return true
	}
	return false
}

// runProgram запускает одну программу с потоковым выводом stdout/stderr
// в консоль. Возвращает ошибку, если выполнение завершилось неуспешно.
func runProgram(manager *toolchain.Manager, prog toolchain.Executable, args []string) error {
	name, cmdArgs := manager.Command(prog, args)

	cmd := exec.Command(name, cmdArgs...)
	cmd.Dir = "."
	// Прямой потоковый вывод в консоль.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// printProgramHeader выводит заголовок перед запуском программы.
func printProgramHeader(prog toolchain.Executable) {
	fmt.Println()
	i18n.Cyan("=== %s ===", prog.Name())
}

// runPhpLint запускает синтаксическую проверку php -l для PHP-файлов scope.
// Файлов нет — проверка просто пропускается, как и остальные линтеры
// при отсутствии подходящих файлов.
func runPhpLint(manager *toolchain.Manager, programs []toolchain.Executable, scope Scope) {
	files := scope.FilesWithExt(".php")
	if len(files) == 0 {
		i18n.Yellow("No PHP files in scope. Skipping php -l.")
		return
	}

	// Ищем php-рантайм среди установленных программ (он входит в Require
	// phpstan/php-cs-fixer и возвращается вместе с ними из Ensure).
	var php toolchain.Executable
	for _, p := range programs {
		if rt, ok := p.(toolchain.Runtime); ok && rt.Name() == "php" {
			php = p
			break
		}
	}
	if php == nil {
		i18n.Yellow("PHP runtime not available. Skipping php -l.")
		return
	}

	fmt.Println()
	i18n.Cyan("=== php -l ===")
	for _, f := range files {
		if err := runProgram(manager, php, []string{"-l", f}); err != nil {
			i18n.Red("php -l %s finished with error: %v", f, err)
		}
	}
}

// goDirArgs возвращает директории, пригодные для golangci-lint:
// только те, что содержат Go-файлы (пакеты). Директории без .go (например
// корневой "." при изменении README.md или go.mod) отбрасываются, иначе
// golangci-lint завершается с ошибкой "no go files to analyze".
func goDirArgs(dirs []string) []string {
	var result []string
	for _, d := range dirs {
		if hasGoFiles(d) {
			result = append(result, d)
		}
	}
	return result
}

// hasGoFiles проверяет наличие .go-файлов на верхнем уровне директории.
// Недоступные директории считаются не содержащими Go-файлов.
func hasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}

// phpCsFixerConfigNames — стандартные имена конфигов php-cs-fixer,
// которые могут существовать в проекте.
var phpCsFixerConfigNames = []string{
	".php-cs-fixer.php",
	".php-cs-fixer.dist.php",
	".php_cs",
	".php_cs.dist",
}

// phpCsFixerDefaultConfig — содержимое генерируемого конфига php-cs-fixer
// с базовыми правилами форматирования PSR-12.
const phpCsFixerDefaultConfig = `<?php

return (new PhpCsFixer\Config())
    ->setRules([
        '@PSR12' => true,
    ]);` + "\n"

// ensurePhpCsFixerConfig возвращает путь к конфигу php-cs-fixer.
// Сначала ищется стандартный конфиг в проекте (dir) — он приоритетен, чтобы
// уважать настройки проекта. Если конфига в проекте нет, генерируется файл
// .php-cs-fixer.php в папке девконфига (~/dev-config)
// (без конфига возникает ошибка "For multiple paths config parameter is required").
func ensurePhpCsFixerConfig(dir string) string {
	for _, name := range phpCsFixerConfigNames {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	devDir := filepath.Join(home, "dev-config")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		return ""
	}
	path := filepath.Join(devDir, "php-cs-fixer.php")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if err := os.WriteFile(path, []byte(phpCsFixerDefaultConfig), 0o644); err != nil {
		return ""
	}
	return path
}
