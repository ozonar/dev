package run

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"dev/internal/common"
	"dev/internal/i18n"
	"dev/internal/port"
	"dev/internal/toolchain"

	"github.com/fatih/color"
)

// DefaultPort — порт dev-сервера по умолчанию.
const DefaultPort = 8000

// RunOptions содержит опции для запуска проекта
type RunOptions struct {
	Port      int    // Порт для dev-сервера (0 = использовать порт по умолчанию)
	PublicDir string // Публичная директория проекта (передаётся из детектора)
	Version   string // Требуемая версия языка (из детектора), например "8.3"
	Target    string // Target — цель запуска (файл, директория или import path).
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
		port = DefaultPort
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
		return runLaravel(opts, port, runtimePath)
	case "yii":
		return runPHP(opts, port, runtimePath)
	case "go":
		return runGo(opts, runtimePath)
	case "node":
		return runNode(opts, runtimePath)
	case "python":
		return runPython(opts, port, runtimePath)
	default:
		if language == "php" {
			return runPHP(opts, port, runtimePath)
		}
		return fmt.Errorf(i18n.T("unsupported framework: %s"), framework)
	}
}

// runPHP запускает встроенный PHP-сервер. Docroot — явно заданная цель
// запуска, иначе публичная директория из детектора.
func runPHP(opts RunOptions, port int, phpPath string) error {
	docroot := opts.Target
	if docroot == "" {
		docroot = opts.PublicDir
	}
	args := append([]string{}, opts.ExtraPhpArgs...)
	args = append(args, "-S", fmt.Sprintf("localhost:%d", port))
	if docroot != "" {
		args = append(args, "-t", docroot)
	}
	return runAndHandlePortError(phpPath, args, port)
}

// runLaravel запускает Laravel-проект: при явной цели — встроенный
// PHP-сервер с указанным docroot, иначе artisan serve.
func runLaravel(opts RunOptions, port int, phpPath string) error {
	if opts.Target != "" {
		return runPHP(opts, port, phpPath)
	}
	if _, err := os.Stat("artisan"); err != nil {
		return errors.New(i18n.T("artisan not found"))
	}
	args := append([]string{}, opts.ExtraPhpArgs...)
	args = append(args, "artisan", "serve")
	if opts.Port != 0 {
		args = append(args, "--port", strconv.Itoa(opts.Port))
	}
	return runAndHandlePortError(phpPath, args, port)
}

// runGo запускает Go-проект: цель — пакет (.go-файл, директория или import
// path); без цели пакет main определяется автоматически.
func runGo(opts RunOptions, goPath string) error {
	target, err := common.SelectGoTarget(opts.Target, "Select number to run [1]: ")
	if err != nil {
		return err
	}
	cmd := exec.Command(goPath, "run", target)
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runNode запускает npm-скрипт проекта. Цель — имя скрипта; без цели
// скрипт выбирается из package.json.
func runNode(opts RunOptions, npmPath string) error {
	script := opts.Target
	if script == "" {
		var err error
		script, err = chooseNpmScript("package.json")
		if err != nil {
			return err
		}
	} else if err := validateNpmScript("package.json", script); err != nil {
		return err
	}
	cmd := exec.Command(npmPath, "run", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runPython запускает Django/Flask проект либо простой HTTP-сервер.
// Для http.server цель — обслуживаемая директория.
func runPython(opts RunOptions, port int, pythonPath string) error {
	if _, err := os.Stat("manage.py"); err == nil {
		if opts.Target != "" {
			return fmt.Errorf(i18n.T("target is not supported for %s projects"), "Django")
		}
		args := []string{"manage.py", "runserver"}
		if opts.Port != 0 {
			args = append(args, strconv.Itoa(opts.Port))
		}
		return runAndHandlePortError(pythonPath, args, port)
	}
	docroot := opts.Target
	if docroot == "" {
		docroot = opts.PublicDir
	}
	args := []string{"-m", "http.server", strconv.Itoa(port)}
	if docroot != "" {
		args = append(args, "--directory", docroot)
	}
	return runAndHandlePortError(pythonPath, args, port)
}

// readNpmScripts читает имена скриптов из package.json.
func readNpmScripts(pkgPath string) (map[string]string, error) {
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil, errors.New(i18n.T("package.json not found"))
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf(i18n.T("error parsing package.json: %v"), err)
	}
	if len(pkg.Scripts) == 0 {
		return nil, errors.New(i18n.T("No npm scripts found in package.json"))
	}
	return pkg.Scripts, nil
}

// validateNpmScript проверяет, что указанный скрипт объявлен в package.json.
func validateNpmScript(pkgPath, name string) error {
	scripts, err := readNpmScripts(pkgPath)
	if err != nil {
		return err
	}
	if _, ok := scripts[name]; !ok {
		return fmt.Errorf(i18n.T("npm script not found: %s"), name)
	}
	return nil
}

// chooseNpmScript читает package.json и возвращает имя npm-скрипта для запуска
// проекта. Если скриптов несколько — показывает нумерованный список и
// спрашивает пользователя; по умолчанию выбирается dev, если он есть,
// иначе первый скрипт в алфавитном порядке.
func chooseNpmScript(pkgPath string) (string, error) {
	scripts, err := readNpmScripts(pkgPath)
	if err != nil {
		return "", err
	}

	names := make([]string, 0, len(scripts))
	for name := range scripts {
		names = append(names, name)
	}
	sort.Strings(names)

	// По умолчанию предпочитаем dev — он запускает dev-сервер проекта.
	defaultIdx := 0
	for i, name := range names {
		if name == "dev" {
			defaultIdx = i
			break
		}
	}

	if len(names) > 1 {
		fmt.Println()
		i18n.Printf("Select npm script to run:\n")
		for i, name := range names {
			fmt.Printf("  %d) %s\n", i+1, name)
		}
		i18n.Printf("Select number to run [%d]: ", defaultIdx+1)
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(names) {
				defaultIdx = n - 1
			}
		}
	}
	return names[defaultIdx], nil
}

// runSymfony запускает Symfony-проект.
// Сначала пробует использовать Symfony CLI (symfony serve),
// если она недоступна — использует php -S с публичной директорией проекта.
// phpPath — путь к рантайму PHP (системный или скачанный в dev-command).
func runSymfony(opts RunOptions, port int, phpPath string) error {
	// При отладке (ExtraPhpArgs) или явно заданной цели используем встроенный
	// PHP-сервер напрямую: Symfony CLI сама управляет процессом php и не
	// передаёт ему наши -d-флаги, а цель запуска у неё задаётся иначе.
	if opts.Target != "" || len(opts.ExtraPhpArgs) > 0 || !isBinaryAvailable("symfony") {
		if opts.Target == "" && len(opts.ExtraPhpArgs) == 0 {
			i18n.Yellow("Symfony CLI not found. Falling back to built-in PHP server (php -S).")
		}
		docroot := opts.Target
		if docroot == "" {
			docroot = opts.PublicDir
		}
		args := append([]string{}, opts.ExtraPhpArgs...)
		args = append(args, "-S", fmt.Sprintf("0.0.0.0:%d", port))
		if docroot != "" {
			args = append(args, "-t", docroot)
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
	occupied, info, err := port.IsPortOccupied(portNum)
	if err != nil {
		return fmt.Errorf(i18n.T("could not check port %d: %v"), portNum, err)
	}
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

	i18n.Green("Process on port %d killed.", portNum)
	return nil
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
