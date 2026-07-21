package run

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"dev/internal/common"

	"github.com/fatih/color"
)

// RunOptions содержит опции для запуска проекта
type RunOptions struct {
	Port int // Порт для dev-сервера (0 = использовать порт по умолчанию)
}

// RunProject запускает проект с опциями по умолчанию
func RunProject(framework, language string) error {
	return RunProjectWithOptions(framework, language, RunOptions{})
}

// RunProjectWithOptions запускает проект с указанными опциями
func RunProjectWithOptions(framework, language string, opts RunOptions) error {
	port := opts.Port
	if port == 0 {
		port = 8000
	}

	switch framework {
	case "symfony":
		args := []string{"serve"}
		if opts.Port != 0 {
			args = append(args, "--port", strconv.Itoa(opts.Port))
		}
		return runAndHandlePortError("symfony", args, port)
	case "laravel":
		// Check if artisan exists
		if _, err := os.Stat("artisan"); err == nil {
			args := []string{"artisan", "serve"}
			if opts.Port != 0 {
				args = append(args, "--port", strconv.Itoa(opts.Port))
			}
			return runAndHandlePortError("php", args, port)
		}
		return fmt.Errorf("artisan not found")
	case "yii":
		// Yii2 — определяем публичную директорию
		publicDir := findYiiPublicDir()
		addr := fmt.Sprintf("localhost:%d", port)
		var args []string
		if publicDir != "" {
			args = []string{"-S", addr, "-t", publicDir}
		} else {
			args = []string{"-S", addr}
		}
		return runAndHandlePortError("php", args, port)
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
			args := []string{"manage.py", "runserver"}
			if opts.Port != 0 {
				args = append(args, strconv.Itoa(opts.Port))
			}
			return runAndHandlePortError("python", args, port)
		}
		// Fallback to simple HTTP server
		addr := strconv.Itoa(port)
		return runAndHandlePortError("python", []string{"-m", "http.server", addr}, port)
	default:
		// Generic PHP server
		if language == "php" {
			addr := fmt.Sprintf("localhost:%d", port)
			return runAndHandlePortError("php", []string{"-S", addr}, port)
		}
		return fmt.Errorf("unsupported framework: %s", framework)
	}
}

// findYiiPublicDir определяет публичную директорию Yii2 проекта
func findYiiPublicDir() string {
	// Yii2 Basic: web/
	if common.FileExists("web/index.php") {
		abs, _ := filepath.Abs("web")
		return abs
	}
	// Yii2 Advanced: frontend/web/
	if common.FileExists("frontend/web/index.php") {
		abs, _ := filepath.Abs("frontend/web")
		return abs
	}
	// Yii2 Advanced: backend/web/
	if common.FileExists("backend/web/index.php") {
		abs, _ := filepath.Abs("backend/web")
		return abs
	}
	// public/ (альтернативный вариант)
	if common.FileExists("public/index.php") {
		abs, _ := filepath.Abs("public")
		return abs
	}
	return ""
}

// runAndHandlePortError запускает команду и при ошибке "address already in use"
// предлагает убить процесс, занимающий порт.
// Создаёт новый exec.Cmd при каждом запуске, чтобы избежать "exec: already started".
func runAndHandlePortError(name string, args []string, port int) error {
	// Первый запуск
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	var stderrBuf strings.Builder
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	err := cmd.Run()
	if err == nil {
		return nil
	}

	// Проверяем и err.Error(), и захваченный stderr
	errStr := err.Error() + "\n" + stderrBuf.String()
	if !isPortInUseError(errStr) {
		return err
	}

	// Порт занят — предлагаем убить процесс
	color.Yellow("⚠ Port %d is already in use.", port)
	fmt.Print("Do you want to kill the process using port " + strconv.Itoa(port) + "? [Y/n]: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "n" || input == "N" || input == "no" || input == "NO" {
		return fmt.Errorf("port %d is already in use", port)
	}

	// Убиваем процесс, занимающий порт
	killCmd := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti:%d -sTCP:LISTEN -t | xargs kill -9 2>/dev/null", port))
	killOutput, _ := killCmd.CombinedOutput()
	if len(killOutput) > 0 {
		fmt.Printf("  %s", string(killOutput))
	}

	// Пробуем ещё раз через fuser (более надёжный)
	fuserCmd := exec.Command("fuser", "-k", fmt.Sprintf("%d/tcp", port))
	fuserCmd.Run()

	color.Green("Process on port %d killed. Restarting...", port)

	// Создаём НОВЫЙ cmd для повторного запуска
	cmd2 := exec.Command(name, args...)
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr
	return cmd2.Run()
}

// isPortInUseError проверяет, содержит ли ошибка сообщение о занятом порте
func isPortInUseError(errStr string) bool {
	lower := strings.ToLower(errStr)
	keywords := []string{
		"address already in use",
		"port already in use",
		"bind: address already in use",
		"only one usage of each socket address",
		"eaddrinuse",
		"addr already in use",
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
