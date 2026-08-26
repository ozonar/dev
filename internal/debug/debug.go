// Пакет debug реализует команду dev debug — запуск проекта под отладчиком.
//
// Для Go используется Delve (dlv), который устанавливается через go install
// (у Delve нет готовых бинарных релизов на GitHub). Аргументы из команды
// {params} передаются отлаживаемой программе после "--".
//
// Для PHP используется локальный Xdebug (php -m): если расширение загружено
// в используемом PHP, сервер запускается сообразно фреймворку проекта
// (Symfony/Laravel/Yii/встроенный PHP-сервер). Без локального Xdebug сервер
// не запускается.
package debug

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"dev/internal/common"
	"dev/internal/port"
	"dev/internal/run"
	"dev/internal/toolchain"

	"github.com/fatih/color"
)

// Options описывает параметры запуска отладки проекта.
type Options struct {
	Framework string   // Фреймворк проекта (symfony, laravel, yii, generic, go)
	Language  string   // Язык проекта (go, php, ...)
	Version   string   // Требуемая версия языка (например "8.3" для PHP)
	PublicDir string   // Публичная директория PHP-проекта
	Params    []string // Дополнительные аргументы, переданные в команду {params}
	Port      int      // Порт: для PHP — dev-сервер (0 = 8000), для Go — DAP-сервер Delve (0 = 2345)
}

// Run запускает отладку проекта в зависимости от языка.
func Run(opts Options) error {
	switch opts.Language {
	case "go":
		return runGo(opts)
	case "php":
		return runPHP(opts)
	default:
		return fmt.Errorf("debug not supported for language %q", opts.Language)
	}
}

// runGo запускает Go-программу под отладчиком Delve.
// Delve устанавливается через go install (для него нет готовых бинарных
// релизов), затем выполняется dlv debug <пакет> -- {params}.
func runGo(opts Options) error {
	goPath, err := toolchain.ResolveRuntime("go", opts.Version)
	if err != nil {
		return err
	}

	// Ищем настоящие main.go (по имени файла и содержимому) в приоритетном
	// порядке cmd/ -> корень. Поиск по имени исключает ложные срабатывания
	// FindGoMain на строковых литералах "package main" в тестовых файлах.
	mainFiles, err := common.FindGoMain(".", common.FindGoMainOptions{
		SearchInCmdFirst: true,
		ExcludeDirs:      []string{"vendor/", "internal/", ".git/"},
		OnlyMainGo:       true,
	})
	if err != nil {
		return fmt.Errorf("error finding main files: %v", err)
	}
	if len(mainFiles) == 0 {
		return fmt.Errorf("no Go main files found")
	}
	target := chooseMain(mainFiles)

	dlvPath, err := installDlv(goPath)
	if err != nil {
		return err
	}

	dapPort := opts.Port
	if dapPort == 0 {
		dapPort = 2345
	}
	if err := ensureDebugPortFree(dapPort); err != nil {
		return err
	}
	dapAddr := fmt.Sprintf("localhost:%d", dapPort)

	dir := debugDir(target)
	color.Green("Starting debug server on %s ...", dapAddr)
	color.Cyan("Waiting for the IDE to attach.")
	fmt.Printf("In your IDE configure an attach debugger (e.g. VS Code launch.json):\n")
	fmt.Printf("  {\"name\":\"attach\",\"type\":\"go\",\"request\":\"attach\",\"mode\":\"remote\",\"port\":%d,\"host\":\"127.0.0.1\"}\n", dapPort)

	// Headless Delve: сервер ожидает подключения клиента (IDE), а запуск
	// программы выполняет IDE после подключения. Без --continue — иначе
	// быстрая CLI-команда успевает завершиться раньше, чем подключится IDE.
	// Вывод запущенной программы идёт в этот терминал.
	args := goDebugArgs(dapAddr, dir, opts.Params)
	cmd := exec.Command(dlvPath, args...)
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// chooseMain выбирает main-файл для отладки. При единственном файле он
// возвращается сразу; при нескольких — предлагается интерактивный выбор.
func chooseMain(mainFiles []string) string {
	if len(mainFiles) == 1 {
		return mainFiles[0]
	}
	fmt.Println("Multiple main files found:")
	for i, f := range mainFiles {
		fmt.Printf("  %d) %s\n", i+1, f)
	}
	fmt.Printf("Select number to debug [1]: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		fmt.Printf("Debugging %s\n", mainFiles[0])
		return mainFiles[0]
	}
	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(mainFiles) {
		return mainFiles[0]
	}
	return mainFiles[idx-1]
}

// goDebugArgs формирует аргументы для dlv debug в headless-режиме.
// Сервер ожидает подключения IDE (без --continue), а запуск программы
// выполняет IDE после attach. Аргументы {params} передаются отлаживаемой
// программе после "--".
func goDebugArgs(dapAddr, target string, params []string) []string {
	args := []string{
		"debug",
		"--headless",
		"--listen=" + dapAddr,
		"--api-version=2",
		"--accept-multiclient",
		"--allow-non-terminal-interactive=true",
		target,
	}
	if len(params) > 0 {
		args = append(args, "--")
		args = append(args, params...)
	}
	return args
}

// debugDir превращает каталог main-файла в относительный путь, понятный Delve.
// Delve интерпретирует путь без префикса "./" как импорт-путь из GOPATH/GOROOT
// (например "cmd/dev" -> "package cmd/dev is not in std"). Поэтому к подпапкам
// добавляется "./". Корень проекта остаётся точкой.
func debugDir(target string) string {
	dir := filepath.Dir(target)
	if dir == "." || dir == "" {
		return "."
	}
	// Убираем возможный ведущий "./" и заворачиваем каталог в "./",
	// т.к. filepath.Clean("./x") схлопывается обратно в "x".
	dir = strings.TrimPrefix(dir, "./")
	return "./" + dir
}

// ensureDebugPortFree проверяет, что DAP-порт свободен. Если он занят
// висящим процессом (например, от предыдущего запуска отладчика), предлагает
// убить его, чтобы избежать ошибки "address already in use".
func ensureDebugPortFree(portNum int) error {
	occupied, info := port.IsPortOccupied(portNum)
	if !occupied {
		return nil
	}

	color.Yellow("Port %d is already in use.", portNum)
	if info != "" {
		fmt.Println(info)
	}
	fmt.Print("Kill the process using port " + strconv.Itoa(portNum) + "? [Y/n]: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "n" || input == "N" || input == "no" || input == "NO" {
		return fmt.Errorf("port %d is already in use", portNum)
	}

	if err := port.KillProcessOnPort(portNum); err != nil {
		color.Yellow("  %v", err)
	}
	fuserCmd := exec.Command("fuser", "-k", fmt.Sprintf("%d/tcp", portNum))
	fuserCmd.Run()

	color.Green("Process on port %d killed.", portNum)
	return nil
}

// installDlv гарантирует доступность Delve и возвращает путь к его бинарю.
// Сначала проверяется системный dlv; если его нет — устанавливается через
// go install github.com/go-delve/delve/cmd/dlv@latest.
func installDlv(goPath string) (string, error) {
	if p := lookupSystem("dlv"); p != "" {
		color.Cyan("Using local Delve: %s", p)
		return p, nil
	}

	color.Yellow("Installing Delve via go install...")
	install := exec.Command(goPath, "install", "github.com/go-delve/delve/cmd/dlv@latest")
	install.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	install.Stdout = os.Stdout
	install.Stderr = os.Stderr
	if err := install.Run(); err != nil {
		return "", fmt.Errorf("failed to install Delve: %v", err)
	}

	bin, err := findDlvBinary(goPath)
	if err != nil {
		return "", err
	}
	color.Green("Delve installed at: %s", bin)
	return bin, nil
}

// findDlvBinary ищет бинарь dlv после установки: в $GOBIN, $GOPATH/bin
// и ~/go/bin.
func findDlvBinary(goPath string) (string, error) {
	gobin, _ := exec.Command(goPath, "env", "GOBIN").Output()
	gopath, _ := exec.Command(goPath, "env", "GOPATH").Output()

	var candidates []string
	if s := strings.TrimSpace(string(gobin)); s != "" {
		candidates = append(candidates, filepath.Join(s, "dlv"))
	}
	if s := strings.TrimSpace(string(gopath)); s != "" {
		candidates = append(candidates, filepath.Join(s, "bin", "dlv"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "go", "bin", "dlv"))
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("dlv binary not found after installation (tried %v)", candidates)
}

// runPHP запускает PHP-сервер сообразно фреймворку проекта. Xdebug используется
// только если он уже загружен в используемом PHP (php -m); запуск сервера
// делегируется dev run — тем самым корректно определяются пути и порт
// (Symfony serve / Laravel artisan / встроенный PHP-сервер). Без локального
// Xdebug сервер не запускается.
func runPHP(opts Options) error {
	phpPath, err := toolchain.ResolveRuntime("php", opts.Version)
	if err != nil {
		return err
	}

	if !phpHasXdebug(phpPath) {
		return fmt.Errorf("Xdebug is not loaded in local PHP (%s). Enable/install Xdebug to debug.", phpPath)
	}
	color.Green("Xdebug detected in local PHP.")

	// Передаём параметры отладки Xdebug напрямую в команду php (-d ...):
	// start_with_request=yes обязателен для CLI/встроенного сервера, иначе
	// брейкпоинты не срабатывают.
	xdebugArgs := []string{
		"-dxdebug.mode=debug",
		"-dxdebug.start_with_request=yes",
		"-dxdebug.client_host=127.0.0.1",
		"-dxdebug.client_port=9003",
	}
	os.Setenv("XDEBUG_MODE", "debug")
	color.Cyan("Xdebug debug mode enabled. Start 'Listening for PHP Debug Connections' in your IDE (port 9003), then open the site.")

	return run.RunProjectWithOptions(opts.Framework, "php", run.RunOptions{
		Port:         opts.Port,
		PublicDir:    opts.PublicDir,
		Version:      opts.Version,
		ExtraPhpArgs: xdebugArgs,
	})
}

// phpHasXdebug проверяет, загружено ли расширение Xdebug в текущем PHP.
func phpHasXdebug(phpPath string) bool {
	out, err := exec.Command(phpPath, "-m").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "xdebug")
}

// lookupSystem ищет исполняемый файл в PATH. Пустая строка означает,
// что инструмент в системе отсутствует.
func lookupSystem(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}
