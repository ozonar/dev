package build

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// BuildProject выполняет сборку проекта в зависимости от фреймворка и языка
func BuildProject(framework, language string) error {
	switch language {
	case "go":
		return buildGo()
	case "javascript":
		return buildNode()
	default:
		fmt.Printf("Сборка для языка %s не требуется\n", language)
		return nil
	}
}

// buildGo собирает Go проект
func buildGo() error {
	// Ищем все main файлы
	mainFiles := findGoMain()
	if len(mainFiles) == 0 {
		return fmt.Errorf("не найден ни один main файл Go")
	}

	var target string
	if len(mainFiles) == 1 {
		target = mainFiles[0]
	} else {
		// Показываем список для выбора
		fmt.Println("Найдено несколько main файлов:")
		for i, f := range mainFiles {
			fmt.Printf("  %d) %s\n", i+1, f)
		}
		fmt.Print("Выберите номер файла для сборки (или 0 для отмены): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" || input == "0" {
			return fmt.Errorf("сборка отменена")
		}
		idx, err := strconv.Atoi(input)
		if err != nil || idx < 1 || idx > len(mainFiles) {
			return fmt.Errorf("неверный выбор")
		}
		target = mainFiles[idx-1]
	}

	// Имя исполняемого файла: если путь содержит cmd/, берём имя поддиректории внутри cmd
	output := outputName(target)

	fmt.Printf("Build %s to %s...\n", target, output)
	cmd := exec.Command("go", "build", "-o", output, target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// buildNode собирает Node.js проект
func buildNode() error {
	if _, err := os.Stat("package.json"); err != nil {
		return fmt.Errorf("package.json не найден")
	}
	// Проверяем, есть ли скрипт build
	cmd := exec.Command("npm", "run", "build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("Запуск npm run build...")
	return cmd.Run()
}

// findGoMain ищет Go файлы с package main и func main
// 1. Сначала ищет в директории cmd/
// 2. Если не находит, ищет по всему проекту файлы с именем main.go
func findGoMain() []string {
	var mains []string

	// Шаг 1: поиск в cmd/
	if _, err := os.Stat("cmd"); err == nil {
		filepath.Walk("cmd", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if strings.HasSuffix(info.Name(), ".go") {
				data, err := os.ReadFile(path)
				if err != nil {
					return nil
				}
				content := string(data)
				if strings.Contains(content, "package main") && strings.Contains(content, "func main") {
					mains = append(mains, path)
				}
			}
			return nil
		})
	}

	// Если нашли в cmd, возвращаем
	if len(mains) > 0 {
		return mains
	}

	// Шаг 2: поиск всех main.go файлов в проекте (кроме vendor, internal)
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Пропускаем служебные директории
		if strings.Contains(path, "vendor/") || strings.Contains(path, "internal/") || strings.Contains(path, ".git/") {
			return nil
		}
		if info.Name() == "main.go" {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			content := string(data)
			if strings.Contains(content, "package main") && strings.Contains(content, "func main") {
				mains = append(mains, path)
			}
		}
		return nil
	})

	return mains
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
