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

	// 0. Create .gitignore если его нет
	if !common.FileExists(".gitignore") {
		actions = append(actions, Action{
			Name:        "create .gitignore",
			Description: "Create .gitignore file for the project",
			Status:      StatusPending,
			Run: func() error {
				return createGitignore(framework, language)
			},
		})
	}

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
			Status:      boolToStatus(isChmodSet("storage", 0777)),
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
			Status:      boolToStatus(isChmodSet("var", 0777)),
			Run: func() error {
				return chmodRecursive("var", 0777)
			},
		})
	}

	// 5.1. symfony server:ca:install (Symfony)
	// Устанавливает локальный центр сертификации для HTTPS в dev-режиме,
	// если он ещё не установлен. Действие добавляется только если Symfony CLI доступен.
	if framework == "symfony" {
		if _, err := exec.LookPath("symfony"); err == nil {
			actions = append(actions, Action{
				Name:        "symfony server:ca:install",
				Description: "Install Symfony local web server TLS certificate authority",
				Status:      boolToStatus(isSymfonyCAInstalled()),
				Run: func() error {
					cmd := exec.Command("symfony", "server:ca:install")
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					return cmd.Run()
				},
			})
		}
	}

	// 6. chmod для Yii2 (runtime/ и web/assets/)
	if framework == "yii" {
		// Yii2 Basic: runtime/
		if common.FileExists("runtime") {
			actions = append(actions, Action{
				Name:        "chmod -R 777 runtime/",
				Description: "Set writable permissions on Yii2 runtime directory",
				Status:      boolToStatus(isChmodSet("runtime", 0777)),
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
					Status:      boolToStatus(isChmodSet(dir, 0777)),
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
					Status:      boolToStatus(isChmodSet(dir, 0777)),
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

	// 10.1. Синхронизация переменных окружения между основным .env и его доп.файлами.
	// Находим доп.файлы к .env (например .env.dist, .env.local), исключая .env.test*,
	// и для каждого создаём действие переноса недостающих переменных.
	if common.FileExists(".env") {
		actions = append(actions, buildEnvSyncActions(".env", findEnvSourceFiles(".", ".env", true))...)
	}
	// Для тестового окружения сравниваем .env.test с его доп.файлами (.env.test*).
	if common.FileExists(".env.test") {
		actions = append(actions, buildEnvSyncActions(".env.test", findEnvSourceFiles(".", ".env.test", false))...)
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
			common.WalkWithExclusions(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				os.Chmod(path, 0777)
				return nil
			}, nil)
			fmt.Printf("  Set 777 on %s\n", dir)
		}
	}
	return nil
}

// chmodRecursive рекурсивно меняет права на директорию
func chmodRecursive(dir string, mode os.FileMode) error {
	return common.WalkWithExclusions(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(path, mode)
	}, nil)
}

// isChmodSet проверяет, установлены ли запрошенные права mode на всей директории
// рекурсивно. Возвращает true, если каждый элемент (файл или подкаталог) внутри dir
// уже имеет права, полностью покрывающие mode (сравнение по маске 0777).
// Если директория не существует или содержит хотя бы один элемент с другими правами,
// возвращается false.
func isChmodSet(dir string, mode os.FileMode) bool {
	// WalkWithExclusions игнорирует ошибки доступа к корню, поэтому
	// существование директории проверяем явно
	if _, err := os.Stat(dir); err != nil {
		return false
	}
	allSet := true
	err := common.WalkWithExclusions(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			allSet = false
			return err
		}
		// Сравниваем только биты прав (0777), игнорируя тип (файл/каталог)
		if info.Mode().Perm()&0777 != mode&0777 {
			allSet = false
			return fmt.Errorf("permissions not set on %s", path)
		}
		return nil
	}, nil)
	if err != nil || !allSet {
		return false
	}
	return true
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

// parseEnvVariables читает файл формата .env и возвращает карту переменных
// (имя → значение). Пропускаются пустые строки и комментарии, начинающиеся с #.
// Из значений снимаются обрамляющие одинарные/двойные кавычки.
func parseEnvVariables(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	vars := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Убираем необязательный префикс export
		line = strings.TrimPrefix(line, "export ")
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		if key == "" {
			continue
		}
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, "\"'")
		vars[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return vars, nil
}

// findEnvSourceFiles возвращает файлы в директории dir, чьи имена начинаются
// с префикса base, исключая сам базовый файл base. Если excludeTest истинно,
// файлы, содержащие в имени подстроку ".test", исключаются.
func findEnvSourceFiles(dir, base string, excludeTest bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == base || !strings.HasPrefix(name, base) {
			continue
		}
		if excludeTest && strings.Contains(name, ".test") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	return files
}

// buildEnvSyncActions строит действия переноса переменных из каждого source-файла
// в целевой файл target. Статус действия:
//   - StatusDone, если все переменные source уже присутствуют в target;
//   - StatusPending, если в target отсутствуют какие-то переменные source.
//
// Run выполняет фактический перенос недостающих переменных из source в target.
func buildEnvSyncActions(target string, sources []string) []Action {
	targetVars, err := parseEnvVariables(target)
	if err != nil {
		return nil
	}
	var actions []Action
	for _, src := range sources {
		srcVars, err := parseEnvVariables(src)
		if err != nil {
			continue
		}
		var missing []string
		for k := range srcVars {
			if _, ok := targetVars[k]; !ok {
				missing = append(missing, k)
			}
		}
		status := StatusDone
		desc := fmt.Sprintf("All variables from %s are already in %s", src, target)
		if len(missing) > 0 {
			status = StatusPending
			desc = fmt.Sprintf("Missing in %s: %s", target, strings.Join(missing, ", "))
		}
		srcCopy := src
		actions = append(actions, Action{
			Name:        fmt.Sprintf("Transfer to %s from %s", target, src),
			Description: desc,
			Status:      status,
			Run: func() error {
				return mergeEnvFile(target, srcCopy)
			},
		})
	}
	return actions
}

// mergeEnvFile дописывает в файл target недостающие переменные из файла source.
// Существующие переменные не перезаписываются; новые добавляются в порядке source.
func mergeEnvFile(target, source string) error {
	targetVars, err := parseEnvVariables(target)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %v", target, err)
	}
	srcVars, err := parseEnvVariables(source)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %v", source, err)
	}
	var lines []string
	for k, v := range srcVars {
		if _, ok := targetVars[k]; !ok {
			lines = append(lines, k+"="+v)
		}
	}
	if len(lines) == 0 {
		return nil
	}
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %v", target, err)
	}
	f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	// Перед добавлением гарантируем наличие перевода строки
	if info.Size() > 0 {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			return err
		}
	}
	return nil
}

// findGenericCacheDirs ищет все директории с именем cache рекурсивно,
// исключая vendor, node_modules, .git и другие системные директории
func findGenericCacheDirs() []string {
	var dirs []string

	common.WalkWithExclusions(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}

		// Если директория называется cache — добавляем
		if info.Name() == "cache" {
			dirs = append(dirs, path)
		}
		return nil
	}, nil)

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

// isSymfonyCAInstalled проверяет, установлен ли локальный центр сертификации Symfony.
// Symfony CLI хранит сертификаты в следующих директориях:
//   - ~/.config/symfony-cli/certs  (современные версии Symfony CLI)
//   - ~/.symfony5/certs            (legacy версии Symfony CLI 5.x)
//
// Наличие файла default.p12 в одной из этих директорий указывает на то,
// что CA-сертификат уже сгенерирован и установлен.
func isSymfonyCAInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	candidates := []string{
		filepath.Join(home, ".config", "symfony-cli", "certs", "default.p12"),
		filepath.Join(home, ".symfony5", "certs", "default.p12"),
	}
	for _, p := range candidates {
		if common.FileExists(p) {
			return true
		}
	}
	return false
}

// gitignoreTemplates возвращает содержимое .gitignore в зависимости от языка/фреймворка
func gitignoreTemplates(framework, language string) string {
	var templates []string

	// Базовые игнорирования для всех проектов
	base := `# Operating System Files
.DS_Store
Thumbs.db
*.swp
*.swo
*~
*.orig

# IDE
.idea/
.vscode/
*.sublime-project
*.sublime-workspace

# Environment
.env
.env.local
.env.*.local

# Logs
*.log

# Dependencies
vendor/
node_modules/
`

	templates = append(templates, base)

	switch framework {
	case "laravel":
		templates = append(templates, `# Laravel
/.phpunit.result.cache
.php_cs.cache
.php-cs-fixer.cache
/build/
/dist/
/bootstrap/cache/*.php
/storage/framework/cache/data/*
/storage/framework/sessions/*
/storage/framework/views/*
/storage/logs/*
!storage/framework/views/.gitkeep
!storage/framework/cache/data/.gitkeep
!storage/framework/sessions/.gitkeep
`)
	case "symfony":
		templates = append(templates, `# Symfony
/.phpunit.result.cache
.php_cs.cache
.php-cs-fixer.cache
/build/
/dist/
/var/
/bootstrap/cache/*.php
`)
	case "node":
		templates = append(templates, `# Node.js
npm-debug.log*
yarn-debug.log*
yarn-error.log*

# Build
/dist/
/build/
*.tsbuildinfo
`)
	case "go":
		templates = append(templates, `# Go
/bin/
/dist/
*.exe
*.exe~
*.dll
*.so
*.dylib
/go.work
/go.work.sum
`)
	case "python":
		templates = append(templates, `# Python
__pycache__/
*.py[cod]
*$py.class
*.so
.Python
venv/
.venv/
env/
*.egg-info/
.installed.cfg
*.egg
dist/
build/
*.whl
MANIFEST
`)
	}

	// Если фреймворк не определён, но язык известен
	if framework == "generic" || framework == "" {
		switch language {
		case "php":
			templates = append(templates, `# PHP
/composer.lock
/.phpunit.result.cache
/build/
/dist/
`)
		case "javascript", "typescript":
			templates = append(templates, `# Node.js
npm-debug.log*
yarn-debug.log*
yarn-error.log*

# Build
/dist/
/build/
*.tsbuildinfo
`)
		case "go":
			templates = append(templates, `# Go
/bin/
/dist/
*.exe
*.exe~
*.dll
*.so
*.dylib
/go.work
/go.work.sum
`)
		case "python":
			templates = append(templates, `# Python
__pycache__/
*.py[cod]
*$py.class
*.so
.Python
venv/
.venv/
env/
*.egg-info/
.installed.cfg
*.egg
dist/
build/
*.whl
MANIFEST
`)
		}
	}

	return strings.Join(templates, "\n")
}

// createGitignore создаёт файл .gitignore на основе языка и фреймворка проекта
func createGitignore(framework, language string) error {
	content := gitignoreTemplates(framework, language)

	if err := os.WriteFile(".gitignore", []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create .gitignore: %v", err)
	}

	fmt.Printf("  Created .gitignore for %s/%s\n", language, framework)
	return nil
}
