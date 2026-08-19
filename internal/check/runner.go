package check

import (
	"dev/internal/toolchain"
	"os"
	"os/exec"
	"strings"

	"github.com/fatih/color"
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
func buildArgs(prog Program, scope Scope, mode Mode) []string {
	var args []string
	switch prog.Name {
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
		if len(dirs) > 0 {
			args = append(args, dirs...)
		} else {
			args = append(args, "./...")
		}
	case "phpstan":
		// PHPStan не поддерживает автоисправление, поэтому режим fix не влияет.
		args = append(args, "analyse", "--memory-limit=1G", "--level=5")
		if len(scope.Files) > 0 {
			args = append(args, scope.Files...)
		} else {
			args = append(args, ".")
		}
	case "php-cs-fixer":
		// php-cs-fixer fix [--dry-run] <paths>. При пустых путях — текущая директория.
		args = append(args, "fix")
		if mode == ModeDryRun {
			args = append(args, "--dry-run")
		}
		if len(scope.Files) > 0 {
			args = append(args, scope.Files...)
		} else {
			args = append(args, ".")
		}
	case "biome":
		// biome check [--write] <paths>. Одна команда объединяет lint, format
		// и импорт-сортировку. В dry-run без --write (только проверка),
		// в fix — с --write (применение исправлений).
		args = append(args, "check")
		if mode == ModeFix {
			args = append(args, "--write")
		}
		if len(scope.Files) > 0 {
			args = append(args, scope.Files...)
		} else {
			args = append(args, ".")
		}
	case "ruff":
		// ruff check [--fix] <paths>. Линтинг и импорт-сортировка.
		// В dry-run без --fix (только проверка), в fix — с --fix.
		args = append(args, "check")
		if mode == ModeFix {
			args = append(args, "--fix")
		}
		if len(scope.Files) > 0 {
			args = append(args, scope.Files...)
		} else {
			args = append(args, ".")
		}
	default:
		// Неизвестная программа — передаём пути как есть.
		args = append(args, scope.Files...)
	}
	return args
}

// runProgram запускает одну программу с потоковым выводом stdout/stderr
// в консоль. Возвращает ошибку, если выполнение завершилось неуспешно.
func runProgram(manager *toolchain.Manager, prog Program, args []string) error {
	name, cmdArgs := manager.Command(prog, args)

	cmd := exec.Command(name, cmdArgs...)
	cmd.Dir = "."
	// Прямой потоковый вывод в консоль.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// printProgramHeader выводит заголовок перед запуском программы.
func printProgramHeader(prog Program) {
	color.Cyan("\n=== %s ===\n", prog.Name)
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
