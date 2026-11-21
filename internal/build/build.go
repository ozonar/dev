package build

import (
	"dev/internal/common"
	"dev/internal/i18n"
	"dev/internal/toolchain"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuildProject выполняет сборку проекта в зависимости от фреймворка и языка.
// target — необязательный явно указанный путь к конкретному файлу
// или директории. При пустом значении цель определяется
// автоматически по main-файлам проекта.
func BuildProject(framework, language, version, output, target string) error {
	switch language {
	case "go":
		return buildGo(version, output, target)
	case "javascript":
		return buildNode()
	default:
		i18n.Printf("Build not required for language %s\n", language)
		return nil
	}
}

// buildGo собирает Go проект.
func buildGo(version, output, target string) error {
	runtimePath, err := toolchain.ResolveRuntime("go", version)
	if err != nil {
		return err
	}

	target, err = common.SelectGoTarget(target, "Select number to build [1]: ")
	if err != nil {
		return err
	}

	// Имя исполняемого файла: явно заданное через -o, либо выведенное из цели
	output = resolveOutput(target, output)

	i18n.Printf("Build %s to %s...\n", target, output)
	cmd := exec.Command(runtimePath, "build", "-o", output, target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	// После успешной сборки выводим полный путь к сгенерированному исполняемому файлу
	absPath, err := filepath.Abs(output)
	if err != nil {
		return fmt.Errorf(i18n.T("error resolving absolute path: %v"), err)
	}
	i18n.Printf("Executable file: %s\n", absPath)
	return nil
}

// buildNode собирает Node.js проект
func buildNode() error {
	if _, err := os.Stat("package.json"); err != nil {
		return errors.New(i18n.T("package.json not found"))
	}
	// Проверяем, есть ли скрипт build
	cmd := exec.Command("npm", "run", "build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	i18n.Printf("Running npm run build...\n")
	return cmd.Run()
}

// outputName возвращает имя выходного файла на основе цели сборки.
func outputName(target string) string {
	// Сборка пакета в корне проекта: имя бинарника по умолчанию.
	if filepath.Clean(target) == "." {
		return "main"
	}
	// Если путь содержит cmd/, берём поддиректорию внутри cmd
	if strings.Contains(target, "cmd/") {
		parts := strings.Split(target, "/")
		for i, part := range parts {
			if part == "cmd" && i+1 < len(parts) {
				// Берём следующую часть после cmd
				return parts[i+1]
			}
		}
	}
	// Иначе берём имя файла без расширения
	base := filepath.Base(target)
	return strings.TrimSuffix(base, ".go")
}

// resolveOutput возвращает имя выходного файла: явно заданное через -o,
// либо выведенное из цели сборки.
func resolveOutput(target, output string) string {
	if output != "" {
		return output
	}
	return outputName(target)
}
