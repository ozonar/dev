package run

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"dev/internal/common"
	"dev/internal/i18n"
	"dev/internal/port"
	"dev/internal/toolchain"

	"github.com/fatih/color"
)

// RunOptions содержит опции для запуска проекта
type RunOptions struct {
	Port      int    // Порт для dev-сервера (0 = использовать порт по умолчанию)
	PublicDir string // Публичная директория проекта (передаётся из детектора)
	Version   string // Требуемая версия языка (из детектора), например "8.3"
	// ExtraPhpArgs — дополнительные аргументы php
	// добавляются в начало команды запуска PHP-сервера.
	ExtraPhpArgs []string
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

	// Определяем путь к рантайму до запуска: системный инструмент, либо
	// скачанный в dev-command.
	runtimePath, err := toolchain.ResolveRuntime(language, opts.Version)
	if err != nil {
		return err
	}

	switch framework {
	case "symfony":
		return runSymfony(opts, port, runtimePath)
	case "laravel":
		// Check if artisan exists
		if _, err := os.Stat("artisan"); err == nil {
			args := append([]string{}, opts.ExtraPhpArgs...)
			args = append(args, "artisan", "serve")
			if opts.Port != 0 {
				args = append(args, "--port", strconv.Itoa(opts.Port))
			}
			return runAndHandlePortError(runtimePath, args, port)
		}
		return fmt.Errorf(i18n.T("artisan not found"))
	case "yii":
		addr := fmt.Sprintf("localhost:%d", port)
		args := append([]string{}, opts.ExtraPhpArgs...)
		if opts.PublicDir != "" {
			args = append(args, "-S", addr, "-t", opts.PublicDir)
		} else {
			args = append(args, "-S", addr)
		}
		return runAndHandlePortError(runtimePath, args, port)
	case "go":
		// Find main.go in cmd/ or root
		mainFiles, err := common.FindGoMain(".", common.FindGoMainOptions{
			SearchInCmdFirst: false,
			ExcludeDirs:      []string{},
			OnlyMainGo:       false,
		})
		if err != nil {
			return fmt.Errorf(i18n.T("error finding main files: %v"), err)
		}
		if len(mainFiles) == 0 {
			return fmt.Errorf(i18n.T("no Go main files found"))
		}
		var target string
		if len(mainFiles) == 1 {
			target = mainFiles[0]
		} else {
			// Показываем список для выбора пользователем
			i18n.Printf("Multiple main files found:\n")
			for i, f := range mainFiles {
				fmt.Printf("  %d) %s\n", i+1, f)
			}
			i18n.Printf("Select number to run [1]: ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" {
				target = mainFiles[0]
				i18n.Printf("Running %s\n", target)
			} else {
				idx, err := strconv.Atoi(input)
				if err != nil || idx < 1 || idx > len(mainFiles) {
					return fmt.Errorf(i18n.T("invalid selection"))
				}
				target = mainFiles[idx-1]
			}
		}
		cmd := exec.Command(runtimePath, "run", target)
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	case "node":
		// Check for package.json scripts
		if _, err := os.Stat("package.json"); err == nil {
			cmd := exec.Command(runtimePath, "run", "dev")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		return fmt.Errorf(i18n.T("package.json not found"))
	case "python":
		// Try to run Django or Flask
		if _, err := os.Stat("manage.py"); err == nil {
			args := []string{"manage.py", "runserver"}
			if opts.Port != 0 {
				args = append(args, strconv.Itoa(opts.Port))
			}
			return runAndHandlePortError(runtimePath, args, port)
		}
		// Fallback to simple HTTP server
		addr := strconv.Itoa(port)
		return runAndHandlePortError(runtimePath, []string{"-m", "http.server", addr}, port)
	default:
		// Generic PHP server
		if language == "php" {
			addr := fmt.Sprintf("localhost:%d", port)
			args := append([]string{}, opts.ExtraPhpArgs...)
			args = append(args, "-S", addr)
			return runAndHandlePortError(runtimePath, args, port)
		}
		return fmt.Errorf(i18n.T("unsupported framework: %s"), framework)
	}
}

// runSymfony запускает Symfony-проект.
// Сначала пробует использовать Symfony CLI (symfony serve),
// если она недоступна — использует php -S с публичной директорией проекта.
// phpPath — путь к рантайму PHP (системный или скачанный в dev-command).
func runSymfony(opts RunOptions, port int, phpPath string) error {
	// При отладке (ExtraPhpArgs) используем встроенный PHP-сервер напрямую:
	// Symfony CLI сама управляет процессом php и не передаёт ему наши -d-флаги.
	if len(opts.ExtraPhpArgs) > 0 || !isBinaryAvailable("symfony") {
		if len(opts.ExtraPhpArgs) == 0 {
			i18n.Yellow("Symfony CLI not found. Falling back to built-in PHP server (php -S).")
		}
		addr := fmt.Sprintf("0.0.0.0:%d", port)
		args := append([]string{}, opts.ExtraPhpArgs...)
		args = append(args, "-S", addr)
		if opts.PublicDir != "" {
			args = append(args, "-t", opts.PublicDir)
		}
		return runAndHandlePortError(phpPath, args, port)
	}

	// Приоритетный способ — Symfony CLI, если она установлена
	args := []string{"serve", "--allow-all-ip"}
	if opts.Port != 0 {
		args = append(args, "--port", strconv.Itoa(opts.Port))
	}
	return runAndHandlePortError("symfony", args, port)
}

// isBinaryAvailable проверяет, доступен ли исполняемый файл в PATH.
func isBinaryAvailable(name string) bool {
	if name == "" {
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}

// runAndHandlePortError запускает команду.
// Перед запуском проактивно проверяет, не занят ли порт висящим сервером,
// и если занят — предлагает убить процесс, чтобы избежать ситуации
// "The local web server is already running".
// Создаёт новый exec.Cmd при каждом запуске, чтобы избежать "exec: already started".
func runAndHandlePortError(name string, args []string, portNum int) error {
	// Проактивно освобождаем порт от висящего сервера перед запуском
	if err := ensurePortFree(portNum); err != nil {
		return err
	}

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
	if isPortInUseError(errStr) {
		// Порт занят процессом, появившимся между проактивной проверкой и запуском.
		// Убиваем его без лишних вопросов и запускаем заново.
		if err := port.KillProcessOnPort(portNum); err != nil {
			color.Yellow("  %v", err)
		}
		killByFuser(portNum)

		i18n.Green("Process on port %d killed. Restarting...", portNum)

		// Создаём НОВЫЙ cmd для повторного запуска
		cmd2 := exec.Command(name, args...)
		cmd2.Stdout = os.Stdout
		cmd2.Stderr = os.Stderr
		return cmd2.Run()
	}
	return err
}

// ensurePortFree проверяет, занят ли порт, и при необходимости предлагает
// убить висящий процесс перед запуском сервера.
func ensurePortFree(portNum int) error {
	occupied, info := port.IsPortOccupied(portNum)
	if !occupied {
		return nil
	}

	i18n.Yellow("⚠ Port %d is already in use.", portNum)
	if info != "" {
		fmt.Println(info)
	}
	i18n.Printf("Do you want to kill the process using port %d? [Y/n]: ", portNum)

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "n" || input == "N" || input == "no" || input == "NO" {
		return fmt.Errorf(i18n.T("port %d is already in use"), portNum)
	}

	if err := port.KillProcessOnPort(portNum); err != nil {
		color.Yellow("  %v", err)
	}
	// Дополнительно пробуем через fuser для надёжности (если он установлен)
	killByFuser(portNum)

	i18n.Green("Process on port %d killed.", portNum)
	return nil
}

// killByFuser завершает процесс на порту через fuser, если он установлен.
// На macOS fuser отсутствует, поэтому вызов безопасно пропускается —
// основное убийство процесса выполняется через lsof (port.KillProcessOnPort).
func killByFuser(portNum int) {
	if _, err := exec.LookPath("fuser"); err != nil {
		return
	}
	_ = exec.Command("fuser", "-k", fmt.Sprintf("%d/tcp", portNum)).Run()
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
