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
	case "symfony", "laravel", "generic":
		// PHP frameworks
		if _, err := os.Stat("bin/console"); err == nil {
			cmd := exec.Command("php", "bin/console", "cache:clear")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		// Fallback to removing directories
		os.RemoveAll("var/cache")
		os.RemoveAll("storage/framework/cache")
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
		// Remove __pycache__ and *.pyc
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
