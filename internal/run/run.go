package run

import (
	"dev/internal/common"
	"fmt"
	"os"
	"os/exec"
)

func RunProject(framework, language string) error {
	switch framework {
	case "symfony":
		cmd := exec.Command("symfony", "serve")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	case "laravel":
		// Check if artisan exists
		if _, err := os.Stat("artisan"); err == nil {
			cmd := exec.Command("php", "artisan", "serve")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		return fmt.Errorf("artisan not found")
	case "go":
		// Find main.go in cmd/ or root
		mainFiles, err := common.FindGoMain(".", common.FindGoMainOptions{
			SearchInCmdFirst: false,
			ExcludeDirs:      []string{},
			OnlyMainGo:       false,
		})
		if err != nil {
			return fmt.Errorf("ошибка поиска main файлов: %v", err)
		}
		if len(mainFiles) == 0 {
			return fmt.Errorf("no Go main files found")
		}
		var target string
		if len(mainFiles) == 1 {
			target = mainFiles[0]
		} else {
			// Let user choose (simplified: pick first)
			target = mainFiles[0]
			fmt.Printf("Multiple main files found, running %s\n", target)
		}
		cmd := exec.Command("go", "run", target)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	case "node":
		// Check for package.json scripts
		if _, err := os.Stat("package.json"); err == nil {
			cmd := exec.Command("npm", "run", "dev")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		return fmt.Errorf("package.json not found")
	case "python":
		// Try to run Django or Flask
		if _, err := os.Stat("manage.py"); err == nil {
			cmd := exec.Command("python", "manage.py", "runserver")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		// Fallback to simple HTTP server
		cmd := exec.Command("python", "-m", "http.server", "8000")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	default:
		// Generic PHP server
		if language == "php" {
			cmd := exec.Command("php", "-S", "localhost:8000")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		return fmt.Errorf("unsupported framework: %s", framework)
	}
}
