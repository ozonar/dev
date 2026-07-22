//go:build !windows

package prepare

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"dev/internal/colors"
	"dev/internal/common"
)

// ActionStatus представляет статус действия
type ActionStatus int

const (
	StatusPending ActionStatus = iota // [ ] — доступно для выполнения
	StatusDone                        // [+] — выполнено
	StatusSkip                        // [-] — пропущено/неактуально
)

// Action представляет одно действие в интерактивном списке
type Action struct {
	Name        string
	Description string
	Status      ActionStatus
	Run         func() error
}

// StatusString возвращает строковое представление статуса
func (a *Action) StatusString() string {
	switch a.Status {
	case StatusDone:
		return colors.Green("+")
	case StatusSkip:
		return colors.Yellow("-")
	default:
		return " "
	}
}

// PrepareProject запускает интерактивный список действий для подготовки проекта
func PrepareProject(framework, language string) error {
	actions := buildActions(framework, language)

	if len(actions) == 0 {
		fmt.Println(colors.Yellow("No actions available for this project."))
		return nil
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		printActions(actions)

		fmt.Print("\nEnter action number to run (or 0/q to exit): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "0" || input == "" {
			break
		}

		idx, err := strconv.Atoi(input)
		if err != nil || idx < 1 || idx > len(actions) {
			fmt.Println(colors.Red("Invalid selection. Please enter a number from the list."))
			continue
		}

		action := &actions[idx-1]

		if action.Status == StatusDone {
			fmt.Println(colors.Yellow("Action already completed. Skipping."))
			continue
		}

		fmt.Printf("\n%s %s...\n", colors.Cyan("▶"), action.Name)
		err = action.Run()
		if err != nil {
			fmt.Printf("%s %s: %v\n", colors.Red("✘"), action.Name, err)
		} else {
			fmt.Printf("%s %s\n", colors.Green("✔"), action.Name)
			action.Status = StatusDone
		}
		fmt.Println()
	}

	fmt.Println(colors.Green("Prepare completed."))
	return nil
}

// buildActions формирует список действий в зависимости от фреймворка и языка
func buildActions(framework, language string) []Action {
	var actions []Action

	// 1. Composer install / npm install / go mod tidy / pip install
	switch framework {
	case "laravel", "symfony", "generic":
		if _, err := os.Stat("composer.json"); err == nil {
			hasVendor := common.FileExists("vendor")
			actions = append(actions, Action{
				Name:        "composer install",
				Description: "Install PHP dependencies via Composer",
				Status:      boolToStatus(hasVendor),
				Run: func() error {
					cmd := exec.Command("composer", "install", "--no-interaction")
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					return cmd.Run()
				},
			})
			actions = append(actions, Action{
				Name:        "composer update",
				Description: "Update PHP dependencies via Composer",
				Status:      StatusPending,
				Run: func() error {
					cmd := exec.Command("composer", "update", "--no-interaction")
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					return cmd.Run()
				},
			})
		}
	case "node":
		if _, err := os.Stat("package.json"); err == nil {
			hasModules := common.FileExists("node_modules")
			actions = append(actions, Action{
				Name:        "npm install",
				Description: "Install Node.js dependencies",
				Status:      boolToStatus(hasModules),
				Run: func() error {
					cmd := exec.Command("npm", "install")
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					return cmd.Run()
				},
			})
		}
	case "go":
		actions = append(actions, Action{
			Name:        "go mod tidy",
			Description: "Tidy Go module dependencies",
			Status:      StatusPending,
			Run: func() error {
				cmd := exec.Command("go", "mod", "tidy")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				return cmd.Run()
			},
		})
	case "python":
		if _, err := os.Stat("requirements.txt"); err == nil {
			pipCmd := findPip()
			actions = append(actions, Action{
				Name:        "pip install",
				Description: "Install Python dependencies from requirements.txt",
				Status:      StatusPending,
				Run: func() error {
					if pipCmd == "" {
						return fmt.Errorf("pip not found. Activate a virtual environment or install pip")
					}
					cmd := exec.Command(pipCmd, "install", "-r", "requirements.txt")
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					return cmd.Run()
				},
			})
		}
	}

	// 2. python -m venv venv (если Python и нет виртуального окружения)
	if language == "python" {
		if !hasVirtualEnv() {
			actions = append(actions, Action{
				Name:        "python -m venv venv",
				Description: "Create Python virtual environment",
				Status:      StatusPending,
				Run: func() error {
					cmd := exec.Command("python3", "-m", "venv", "venv")
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					return cmd.Run()
				},
			})
		}
	}

	// 3. npm run build (если Node.js и есть build скрипт в package.json)
	if framework == "node" || language == "javascript" {
		if hasNpmBuildScript() {
			actions = append(actions, Action{
				Name:        "npm run build",
				Description: "Build frontend assets",
				Status:      StatusPending,
				Run: func() error {
					cmd := exec.Command("npm", "run", "build")
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					return cmd.Run()
				},
			})
		}
	}

	// 4. chmod -R 777 storage/ (Laravel)
	if framework == "laravel" && common.FileExists("storage") {
		actions = append(actions, Action{
			Name:        "chmod -R 777 storage/",
			Description: "Set writable permissions on storage directory",
			Status:      StatusPending,
			Run: func() error {
				return chmodRecursive("storage", 0777)
			},
		})
	}

	// 5. chmod -R 777 var/ (Symfony)
	if framework == "symfony" && common.FileExists("var") {
		actions = append(actions, Action{
			Name:        "chmod -R 777 var/",
			Description: "Set writable permissions on var directory",
			Status:      StatusPending,
			Run: func() error {
				return chmodRecursive("var", 0777)
			},
		})
	}

	// 6. chmod для Yii2 (runtime/ и web/assets/)
	if framework == "yii" {
		// Yii2 Basic: runtime/
		if common.FileExists("runtime") {
			actions = append(actions, Action{
				Name:        "chmod -R 777 runtime/",
				Description: "Set writable permissions on Yii2 runtime directory",
				Status:      StatusPending,
				Run: func() error {
					return chmodRecursive("runtime", 0777)
				},
			})
		}
		// Yii2 Advanced: backend/runtime/, frontend/runtime/
		for _, dir := range []string{"backend/runtime", "frontend/runtime"} {
			if common.FileExists(dir) {
				actions = append(actions, Action{
					Name:        fmt.Sprintf("chmod -R 777 %s/", dir),
					Description: fmt.Sprintf("Set writable permissions on %s", dir),
					Status:      StatusPending,
					Run: func(d string) func() error {
						return func() error {
							return chmodRecursive(d, 0777)
						}
					}(dir),
				})
			}
		}
		// Yii2 Advanced: backend/web/assets/, frontend/web/assets/
		for _, dir := range []string{"backend/web/assets", "frontend/web/assets"} {
			if common.FileExists(dir) {
				actions = append(actions, Action{
					Name:        fmt.Sprintf("chmod -R 777 %s/", dir),
					Description: fmt.Sprintf("Set writable permissions on %s", dir),
					Status:      StatusPending,
					Run: func(d string) func() error {
						return func() error {
							return chmodRecursive(d, 0777)
						}
					}(dir),
				})
			}
		}
	}

	// 7. sudo chown -R www-data:www-data (для storage/, var/, runtime/)
	chownDirs := findChownDirs(framework)
	if len(chownDirs) > 0 {
		alreadyOwned := true
		for _, dir := range chownDirs {
			if !isOwnedByWwwData(dir) {
				alreadyOwned = false
				break
			}
		}
		actions = append(actions, Action{
			Name:        fmt.Sprintf("sudo chown -R www-data:www-data %s/", strings.Join(chownDirs, "/")),
			Description: fmt.Sprintf("Set www-data ownership on %s", strings.Join(chownDirs, ", ")),
			Status:      boolToStatus(alreadyOwned),
			Run: func() error {
				for _, dir := range chownDirs {
					cmd := exec.Command("sudo", "chown", "-R", "www-data:www-data", dir)
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					if err := cmd.Run(); err != nil {
						return fmt.Errorf("chown %s failed: %v", dir, err)
					}
					fmt.Printf("  Changed owner of %s/ to www-data:www-data\n", dir)
				}
				return nil
			},
		})
	}

	// 8. php artisan storage:link (Laravel)
	if framework == "laravel" && common.FileExists("artisan") {
		hasStorageLink := common.FileExists("public/storage")
		actions = append(actions, Action{
			Name:        "php artisan storage:link",
			Description: "Create symbolic link from public/storage to storage/app/public",
			Status:      boolToStatus(hasStorageLink),
			Run: func() error {
				cmd := exec.Command("php", "artisan", "storage:link")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				return cmd.Run()
			},
		})
	}

	// 9. Set cache folder 777 (только для generic PHP — ищем все /cache директории)
	if framework == "generic" {
		cacheDirs := findGenericCacheDirs()
		if len(cacheDirs) > 0 {
			allSet := true
			for _, dir := range cacheDirs {
				if !isWritable(dir) {
					allSet = false
					break
				}
			}
			actions = append(actions, Action{
				Name:        "set cache folder 777",
				Description: fmt.Sprintf("Set 777 permissions on cache directories (%s)", strings.Join(cacheDirs, ", ")),
				Status:      boolToStatus(allSet),
				Run: func() error {
					return setCachePermissions(cacheDirs)
				},
			})
		}
	}

	// 10. Init .env
	envSources := []string{".env.dist", ".env.dev", ".env.example"}
	hasEnv := common.FileExists(".env")
	hasEnvSource := false
	for _, src := range envSources {
		if common.FileExists(src) {
			hasEnvSource = true
			break
		}
	}
	if hasEnvSource {
		actions = append(actions, Action{
			Name:        "init .env",
			Description: "Copy .env.dist/.env.dev/.env.example to .env",
			Status:      boolToStatus(hasEnv),
			Run: func() error {
				return copyEnvFiles(envSources)
			},
		})
	}

	// 11. git submodule update --init --recursive
	if common.FileExists(".gitmodules") {
		actions = append(actions, Action{
			Name:        "git submodule update --init --recursive",
			Description: "Initialize and update git submodules",
			Status:      StatusPending,
			Run: func() error {
				cmd := exec.Command("git", "submodule", "update", "--init", "--recursive")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				return cmd.Run()
			},
		})
	}

	// 12. docker compose up -d
	if common.FileExists("docker-compose.yml") {
		actions = append(actions, Action{
			Name:        "docker compose up -d",
			Description: "Start Docker Compose services in background",
			Status:      StatusPending,
			Run: func() error {
				cmd := exec.Command("docker-compose", "up", "-d")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				return cmd.Run()
			},
		})
		// 13. rebuild docker compose
		actions = append(actions, Action{
			Name:        "rebuild docker compose",
			Description: "Rebuild and restart Docker Compose services",
			Status:      StatusPending,
			Run: func() error {
				fmt.Println("Stopping containers...")
				stopCmd := exec.Command("docker-compose", "down")
				stopCmd.Stdout = os.Stdout
				stopCmd.Stderr = os.Stderr
				if err := stopCmd.Run(); err != nil {
					return fmt.Errorf("docker-compose down failed: %v", err)
				}

				fmt.Println("Building and starting containers...")
				upCmd := exec.Command("docker-compose", "up", "-d", "--build")
				upCmd.Stdout = os.Stdout
				upCmd.Stderr = os.Stderr
				if err := upCmd.Run(); err != nil {
					return fmt.Errorf("docker-compose up failed: %v", err)
				}

				return nil
			},
		})
	}

	return actions
}

// printActions выводит список действий
func printActions(actions []Action) {
	fmt.Println(colors.Cyan("\n=== Available Actions ==="))
	for i, a := range actions {
		statusStr := a.StatusString()
		desc := ""
		if a.Description != "" {
			desc = colors.Gray(" (" + a.Description + ")")
		}
		fmt.Printf("[%s] %d. %s%s\n", statusStr, i+1, a.Name, desc)
	}
}

// setCachePermissions устанавливает права 777 на директории кэша
func setCachePermissions(dirs []string) error {
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err == nil {
			if err := os.Chmod(dir, 0777); err != nil {
				return fmt.Errorf("failed to chmod %s: %v", dir, err)
			}
			filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				os.Chmod(path, 0777)
				return nil
			})
			fmt.Printf("  Set 777 on %s\n", dir)
		}
	}
	return nil
}

// chmodRecursive рекурсивно меняет права на директорию
func chmodRecursive(dir string, mode os.FileMode) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(path, mode)
	})
}

// copyEnvFiles копирует .env.dist/.env.dev/.env.example в .env
func copyEnvFiles(sources []string) error {
	for _, src := range sources {
		if _, err := os.Stat(src); err == nil {
			data, err := os.ReadFile(src)
			if err != nil {
				return fmt.Errorf("failed to read %s: %v", src, err)
			}
			if err := os.WriteFile(".env", data, 0644); err != nil {
				return fmt.Errorf("failed to write .env: %v", err)
			}
			fmt.Printf("  Copied %s → .env\n", src)
			return nil
		}
	}
	return fmt.Errorf("no .env source file found")
}

// findGenericCacheDirs ищет все директории с именем cache рекурсивно,
// исключая vendor, node_modules, .git и другие системные директории
func findGenericCacheDirs() []string {
	var dirs []string
	excludeDirs := map[string]bool{
		"vendor":       true,
		"node_modules": true,
		".git":         true,
		".venv":        true,
		"venv":         true,
		"env":          true,
		"__pycache__":  true,
	}

	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}

		// Пропускаем исключённые директории
		base := filepath.Base(path)
		if excludeDirs[base] && path != "." {
			return filepath.SkipDir
		}

		// Если директория называется cache — добавляем
		if info.Name() == "cache" {
			dirs = append(dirs, path)
		}
		return nil
	})

	return dirs
}

// isWritable проверяет, доступна ли директория для записи
func isWritable(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return info.Mode().Perm()&0200 != 0
}

// findPip ищет pip в виртуальном окружении или системный
func findPip() string {
	venvPips := []string{
		".venv/bin/pip",
		"venv/bin/pip",
		"env/bin/pip",
	}
	for _, p := range venvPips {
		if common.FileExists(p) {
			return p
		}
	}
	if venv := os.Getenv("VIRTUAL_ENV"); venv != "" {
		pipPath := filepath.Join(venv, "bin", "pip")
		if common.FileExists(pipPath) {
			return pipPath
		}
	}
	if _, err := exec.LookPath("pip"); err == nil {
		return "pip"
	}
	if _, err := exec.LookPath("pip3"); err == nil {
		return "pip3"
	}
	return ""
}

// hasVirtualEnv проверяет наличие виртуального окружения Python
func hasVirtualEnv() bool {
	candidates := []string{".venv", "venv", "env"}
	for _, d := range candidates {
		if common.FileExists(d) {
			// Проверяем, что это похоже на venv (есть bin/python)
			pythonBin := filepath.Join(d, "bin", "python")
			if common.FileExists(pythonBin) {
				return true
			}
			// fallback: проверяем pyvenv.cfg
			if common.FileExists(filepath.Join(d, "pyvenv.cfg")) {
				return true
			}
		}
	}
	return false
}

// hasNpmBuildScript проверяет наличие скрипта "build" в package.json
func hasNpmBuildScript() bool {
	data, err := os.ReadFile("package.json")
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}
	_, ok := pkg.Scripts["build"]
	return ok
}

// findChownDirs возвращает список директорий, которые нужно отдать www-data
func findChownDirs(framework string) []string {
	switch framework {
	case "laravel":
		return []string{"storage"}
	case "symfony":
		return []string{"var"}
	case "yii":
		var dirs []string
		for _, d := range []string{"runtime", "backend/runtime", "frontend/runtime", "backend/web/assets", "frontend/web/assets"} {
			if common.FileExists(d) {
				dirs = append(dirs, d)
			}
		}
		return dirs
	default:
		// Проверяем наличие типичных директорий
		var dirs []string
		candidates := []string{"storage", "var", "runtime"}
		for _, d := range candidates {
			if common.FileExists(d) {
				dirs = append(dirs, d)
			}
		}
		return dirs
	}
}

// isOwnedByWwwData проверяет, принадлежит ли директория www-data:www-data
func isOwnedByWwwData(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	// www-data обычно имеет UID/GID 33, но не везде
	// Проверяем через stat, сравнивая с ожидаемыми значениями
	// 33 — стандартный UID/GID для www-data в Debian/Ubuntu
	return stat.Uid == 33 && stat.Gid == 33
}

// boolToStatus преобразует bool в ActionStatus
// true → StatusDone, false → StatusPending
func boolToStatus(b bool) ActionStatus {
	if b {
		return StatusDone
	}
	return StatusPending
}
