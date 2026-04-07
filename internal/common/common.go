package common

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FileExists проверяет, существует ли файл или директория по указанному пути.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Unique возвращает срез уникальных строк в порядке первого появления.
func Unique(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// RunCommand запускает команду с аргументами и перенаправляет stdout/stderr в os.Stdout/os.Stderr.
// Возвращает ошибку выполнения команды.
func RunCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunCommandWithOutput запускает команду и возвращает её вывод (stdout) в виде строки.
func RunCommandWithOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("команда %s %v завершилась с ошибкой: %v", name, args, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// FindGoMainOptions параметры для поиска main-файлов Go.
type FindGoMainOptions struct {
	SearchInCmdFirst bool     // сначала искать в cmd/
	ExcludeDirs      []string // директории для исключения (например, "vendor/", "internal/", ".git/")
	OnlyMainGo       bool     // искать только файлы с именем main.go (иначе все .go файлы)
}

// FindGoMain ищет Go файлы, содержащие package main и func main.
// Возвращает пути к найденным файлам относительно корня проекта.
func FindGoMain(root string, opts FindGoMainOptions) ([]string, error) {
	var mains []string

	// Шаг 1: поиск в cmd/ если включено
	if opts.SearchInCmdFirst {
		cmdDir := filepath.Join(root, "cmd")
		if FileExists(cmdDir) {
			err := filepath.Walk(cmdDir, func(path string, info os.FileInfo, err error) error {
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
						rel, _ := filepath.Rel(root, path)
						mains = append(mains, rel)
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
		if len(mains) > 0 {
			return mains, nil
		}
	}

	// Шаг 2: поиск по всему проекту
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Проверка исключений
		rel, _ := filepath.Rel(root, path)
		for _, excl := range opts.ExcludeDirs {
			if strings.Contains(rel, excl) {
				return nil
			}
		}
		// Фильтр по имени файла
		if opts.OnlyMainGo && info.Name() != "main.go" {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		if strings.Contains(content, "package main") && strings.Contains(content, "func main") {
			mains = append(mains, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mains, nil
}
