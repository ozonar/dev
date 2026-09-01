package build

import (
	"bufio"
	"dev/internal/common"
	"dev/internal/toolchain"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// BuildProject выполняет сборку проекта в зависимости от фреймворка и языка.
func BuildProject(framework, language, version string) error {
	switch language {
	case "go":
		return buildGo(version)
	case "javascript":
		return buildNode()
	default:
		fmt.Printf("Build not required for language %s\n", language)
		return nil
	}
}

// buildGo собирает Go проект
func buildGo(version string) error {
	runtimePath, err := toolchain.ResolveRuntime("go", version)
	if err != nil {
		return err
	}
	// Ищем все main файлы
	mainFiles, err := common.FindGoMain(".", common.FindGoMainOptions{
		SearchInCmdFirst: true,
		ExcludeDirs:      []string{"vendor/", "internal/", ".git/"},
		OnlyMainGo:       false,
	})
	if err != nil {
		return fmt.Errorf("error finding main files: %v", err)
	}
	if len(mainFiles) == 0 {
		return fmt.Errorf("no Go main file found")
	}

	var target string
	if len(mainFiles) == 1 {
		target = mainFiles[0]
	} else {
		// Show list for user to choose
		fmt.Println("Multiple main files found:")
		for i, f := range mainFiles {
			fmt.Printf("  %d) %s\n", i+1, f)
		}
		fmt.Printf("Select number to build [1]: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			target = mainFiles[0]
			fmt.Printf("Building %s\n", target)
		} else {
			idx, err := strconv.Atoi(input)
			if err != nil || idx < 1 || idx > len(mainFiles) {
				return fmt.Errorf("invalid selection")
			}
			target = mainFiles[idx-1]
		}
	}

	// Имя исполняемого файла: если путь содержит cmd/, берём имя поддиректории внутри cmd
	output := outputName(target)

	fmt.Printf("Build %s to %s...\n", target, output)
	cmd := exec.Command(runtimePath, "build", "-o", output, target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	// После успешной сборки выводим полный путь к сгенерированному исполняемому файлу
	absPath, err := filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("error resolving absolute path: %v", err)
	}
	fmt.Printf("Executable file: %s\n", absPath)
	return nil
}

// buildNode собирает Node.js проект
func buildNode() error {
	if _, err := os.Stat("package.json"); err != nil {
		return fmt.Errorf("package.json not found")
	}
	// Проверяем, есть ли скрипт build
	cmd := exec.Command("npm", "run", "build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("Running npm run build...")
	return cmd.Run()
}

// outputName возвращает имя выходного файла на основе пути к main.go
func outputName(target string) string {
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
