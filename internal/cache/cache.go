package cache

import (
	"dev/internal/common"
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
		// (с пропуском node_modules, vendor, .git и т.д.)
		if err := common.WalkWithExclusions(".", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && info.Name() == "cache" {
				// Удаляем все файлы и поддиректории внутри директории cache
				entries, err := os.ReadDir(path)
				if err != nil {
					return err
				}
				for _, entry := range entries {
					fullPath := filepath.Join(path, entry.Name())
					if err := os.RemoveAll(fullPath); err != nil {
						return err
					}
				}
			}
			return nil
		}, nil); err != nil {
			return err
		}
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
		// Удаляем __pycache__ и *.pyc файлы (с пропуском node_modules, vendor, .git и т.д.)
		if err := common.WalkWithExclusions(".", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && info.Name() == "__pycache__" {
				if err := os.RemoveAll(path); err != nil {
					return err
				}
			}
			if strings.HasSuffix(info.Name(), ".pyc") {
				if err := os.Remove(path); err != nil {
					return err
				}
			}
			return nil
		}, nil); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported framework: %s", framework)
	}
}
