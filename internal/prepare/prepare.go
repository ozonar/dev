package prepare

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func PrepareProject(framework, language string) error {
	// 1. Set 777 permissions on cache directories
	setCachePermissions()

	// 2. Copy .env.dist or .env.dev to .env
	copyEnvFiles()

	// 3. Install/reinstall vendors
	installVendors(framework, language)

	return nil
}

func setCachePermissions() {
	cacheDirs := []string{
		"var/cache",
		"storage/framework/cache",
		"tmp",
		"cache",
		"__pycache__",
	}
	for _, dir := range cacheDirs {
		if _, err := os.Stat(dir); err == nil {
			os.Chmod(dir, 0777)
			filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				os.Chmod(path, 0777)
				return nil
			})
			fmt.Printf("Set permissions 777 on %s\n", dir)
		}
	}
}

func copyEnvFiles() {
	envSources := []string{".env.dist", ".env.dev", ".env.example"}
	for _, src := range envSources {
		if _, err := os.Stat(src); err == nil {
			data, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			os.WriteFile(".env", data, 0644)
			fmt.Printf("Copied %s to .env\n", src)
			return
		}
	}
	fmt.Println("No .env source found")
}

func installVendors(framework, language string) {
	switch framework {
	case "laravel", "symfony", "generic":
		if _, err := os.Stat("composer.json"); err == nil {
			fmt.Println("Running composer install...")
			cmd := exec.Command("composer", "install", "--no-interaction")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		}
	case "node":
		if _, err := os.Stat("package.json"); err == nil {
			fmt.Println("Running npm install...")
			cmd := exec.Command("npm", "install")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		}
	case "go":
		fmt.Println("Running go mod tidy...")
		cmd := exec.Command("go", "mod", "tidy")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	case "python":
		if _, err := os.Stat("requirements.txt"); err == nil {
			fmt.Println("Running pip install -r requirements.txt...")
			cmd := exec.Command("pip", "install", "-r", "requirements.txt")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		}
	}
}
