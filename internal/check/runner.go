package check

import (
	"os"
	"os/exec"

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
		args = append(args, "run")
		if mode == ModeFix {
			args = append(args, "--fix")
		}
		if len(scope.Dirs) > 0 {
			args = append(args, scope.Dirs...)
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
	default:
		// Неизвестная программа — передаём пути как есть.
		args = append(args, scope.Files...)
	}
	return args
}

// runProgram запускает одну программу с потоковым выводом stdout/stderr
// в консоль. Возвращает ошибку, если выполнение завершилось неуспешно.
func runProgram(dir string, prog Program, args []string) error {
	name, cmdArgs := prog.runCommand(dir, args)

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
