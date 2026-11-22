package port

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"dev/internal/i18n"
	"dev/internal/procfs"

	"github.com/fatih/color"
)

// isLocalHost проверяет, является ли хост локальным
func isLocalHost(host string) bool {
	return host == "127.0.0.1" || host == "localhost" || host == "::1" || host == "0.0.0.0"
}

// IsPortOccupied проверяет, занят ли локальный порт, и возвращает информацию
// о процессах-владельцах.
func IsPortOccupied(port int) (bool, string, error) {
	listeners, err := procfs.Listeners(port)
	if err != nil {
		return false, "", err
	}
	if len(listeners) == 0 {
		return false, "", nil
	}

	lines := make([]string, 0, len(listeners))
	for _, l := range listeners {
		switch {
		case l.PID == 0:
			lines = append(lines, "owner: unknown")
		case l.Name != "":
			lines = append(lines, fmt.Sprintf("PID %d: %s", l.PID, l.Name))
		default:
			lines = append(lines, fmt.Sprintf("PID %d", l.PID))
		}
	}
	return true, strings.Join(lines, "\n"), nil
}

// KillProcessOnPort убивает все процессы, слушающие указанный порт.
// Свободный порт — не ошибка.
func KillProcessOnPort(port int) error {
	return procfs.KillPort(port)
}

// EnsureFree проверяет, свободен ли порт, и при занятости предлагает убить
// процесс, слушающий его.
func EnsureFree(portNum int) error {
	occupied, info, err := IsPortOccupied(portNum)
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
	if strings.EqualFold(input, "n") || strings.EqualFold(input, "no") {
		return fmt.Errorf(i18n.T("port %d is already in use"), portNum)
	}

	if err := KillProcessOnPort(portNum); err != nil {
		color.Yellow("  %v", err)
	}

	i18n.Green("Process on port %d killed.", portNum)
	return nil
}

// CheckPort проверяет, занят ли указанный адрес:порт, и если да — показывает
// информацию о процессе и предлагает выбор: убить процесс или запустить nmap
// для детекции сервиса. По умолчанию выбирается убийство процесса.
// Формат addr: "127.0.0.1:1000", ":8080" или просто "8080"
func CheckPort(addr string) error {
	host, port, err := parseAddr(addr)
	if err != nil {
		return fmt.Errorf(i18n.T("invalid address format %q: %v"), addr, err)
	}

	if !isLocalHost(host) {
		// Для удалённых хостов — сразу запускаем nmap без вопроса
		fmt.Println()
		runNmap(host, port)
		return nil
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf(i18n.T("invalid port %q: %v"), port, err)
	}

	// Для локальных хостов — проверка владельцев порта.
	occupied, procInfo, err := IsPortOccupied(portNum)
	if err != nil {
		return fmt.Errorf(i18n.T("could not check port %s:%s: %v"), host, port, err)
	}
	if !occupied {
		i18n.Printf("Port %s:%s is free\n", host, port)
		return nil
	}

	// Порт занят — показываем детали
	i18n.Printf("Port %s:%s is in use\n", host, port)
	if procInfo != "" {
		fmt.Println(procInfo)
	}

	// Предлагаем выбор: убить процесс или запустить nmap (по умолчанию — убить).
	fmt.Println()
	i18n.Printf("1. Kill the process occupying the port\n")
	i18n.Printf("2. Run nmap\n")
	i18n.Printf("Choose (default 1): ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	switch input {
	case "", "1":
		// По умолчанию и явный выбор "1" — убиваем процесс на порту
		portNum, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf(i18n.T("invalid port %q: %v"), port, err)
		}
		i18n.Printf("Killing process on port %s:%s...\n", host, port)
		if err := KillProcessOnPort(portNum); err != nil {
			return fmt.Errorf(i18n.T("failed to kill process on port %s:%s: %v"), host, port, err)
		}
		i18n.Printf("Process killed\n")
	case "2":
		fmt.Println()
		runNmap(host, port)
	default:
		i18n.Printf("Invalid selection, nothing done\n")
	}

	return nil
}

// parseAddr разбирает адрес на host и port.
// Поддерживает форматы:
//   - "127.0.0.1:1000"
//   - ":8080"
//   - "8080" (только порт, хост = 127.0.0.1)
func parseAddr(addr string) (host, port string, err error) {
	if !strings.Contains(addr, ":") {
		return "127.0.0.1", addr, nil
	}

	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf(i18n.T("expected host:port format, got %q"), addr)
	}
	host = parts[0]
	port = parts[1]
	if host == "" {
		host = "127.0.0.1"
	}
	return host, port, nil
}

// runNmap запускает nmap -sV на указанном порту и выводит результат.
func runNmap(host, port string) {
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		i18n.Printf("invalid port: %s\n", port)
		return
	}

	cmd := exec.Command("nmap", "-sV", "-T4", "-p", port, host)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		i18n.Printf("nmap: %v\n", err)
	}
}
