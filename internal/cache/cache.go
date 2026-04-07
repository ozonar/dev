package cache

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func ClearCache(framework string) error {
	switch framework {
	case "symfony":
		if _, err := os.Stat("bin/console"); err == nil {
			cmd := exec.Command("php", "bin/console", "cache:clear")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		return nil
	case "laravel":
		if _, err := os.Stat("artisan"); err == nil {
			cmd := exec.Command("php", "artisan", "cache:clear")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		return nil
	case "yii":
		// Очистка кэша Yii
		if _, err := os.Stat("yii"); err == nil {
			cmd := exec.Command("php", "yii", "cache/flush-all")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}

		return nil
	case "generic":
		// Ищем любую директорию с именем "cache" и очищаем её содержимое
		filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && info.Name() == "cache" {
				// Удаляем все файлы и поддиректории внутри директории cache
				entries, err := os.ReadDir(path)
				if err != nil {
					return nil
				}
				for _, entry := range entries {
					fullPath := filepath.Join(path, entry.Name())
					os.RemoveAll(fullPath)
				}
			}
			return nil
		})
		return nil
	case "go":
		cmd := exec.Command("go", "clean", "-cache", "-modcache", "-testcache")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	case "node":
		cmd := exec.Command("npm", "cache", "clean", "--force")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	case "python":
		// Удаляем __pycache__ и *.pyc файлы
		filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && info.Name() == "__pycache__" {
				os.RemoveAll(path)
			}
			if strings.HasSuffix(info.Name(), ".pyc") {
				os.Remove(path)
			}
			return nil
		})
		return nil
	default:
		return fmt.Errorf("unsupported framework: %s", framework)
	}
}
