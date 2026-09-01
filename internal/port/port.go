package port

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// isLocalHost проверяет, является ли хост локальным
func isLocalHost(host string) bool {
	return host == "127.0.0.1" || host == "localhost" || host == "::1" || host == "0.0.0.0"
}

// IsPortOccupied проверяет, занят ли локальный порт, и возвращает
// информацию о процессе, если он занят. Переиспользует существующие
// методы детекции (fuser, ss, lsof).
func IsPortOccupied(port int) (bool, string) {
	portStr := strconv.Itoa(port)

	// Последовательно пробуем несколько методов детекции
	occupied, info := checkPortFuser(portStr)
	if !occupied {
		occupied, info = checkPortSS(portStr)
	}
	if !occupied {
		occupied, info = checkPortLsof(portStr)
	}
	return occupied, info
}

// KillProcessOnPort убивает все процессы, занимающие указанный порт.
// Использует lsof для получения PID и kill для завершения.
// Возвращает ошибку, если не удалось найти или убить процесс.
func KillProcessOnPort(port int) error {
	// Получаем PID процессов, слушающих порт, через lsof
	listCmd := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti:%d -sTCP:LISTEN 2>/dev/null", port))
	output, err := listCmd.Output()
	if err != nil && len(output) == 0 {
		// Нет процессов на порту — ничего убивать не нужно
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not find process on port %d: %v", port, err)
	}

	pids := strings.Fields(string(output))
	if len(pids) == 0 {
		return nil
	}

	// Убиваем каждый найденный PID
	args := append([]string{"-9"}, pids...)
	killCmd := exec.Command("kill", args...)
	killCmd.Stdout = os.Stdout
	killCmd.Stderr = os.Stderr
	if err := killCmd.Run(); err != nil {
		return fmt.Errorf("could not kill process on port %d: %v", port, err)
	}
	return nil
}

// CheckPort проверяет, занят ли указанный адрес:порт, и если да — показывает
// информацию о процессе и предлагает запустить nmap для детекции сервиса.
// Формат addr: "127.0.0.1:1000", ":8080" или просто "8080"
func CheckPort(addr string) error {
	host, port, err := parseAddr(addr)
	if err != nil {
		return fmt.Errorf("invalid address format %q: %v", addr, err)
	}

	if !isLocalHost(host) {
		// Для удалённых хостов — сразу запускаем nmap без вопроса
		fmt.Println()
		runNmap(host, port)
		return nil
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port %q: %v", port, err)
	}

	// Для локальных хостов — проверка через fuser/ss/lsof.
	occupied, procInfo := IsPortOccupied(portNum)
	if !occupied {
		fmt.Printf("Port %s:%s is free\n", host, port)
		return nil
	}

	// Порт занят — показываем детали
	fmt.Printf("Port %s:%s is in use\n", host, port)
	if procInfo != "" {
		fmt.Println(procInfo)
	}

	// Спрашиваем, запускать ли nmap (по умолчанию — да).
	fmt.Print("\nWant nmap for port? [Y/n]: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if !strings.EqualFold(input, "n") && !strings.EqualFold(input, "no") {
		fmt.Println()
		runNmap(host, port)
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
		return "", "", fmt.Errorf("expected host:port format, got %q", addr)
	}
	host = parts[0]
	port = parts[1]
	if host == "" {
		host = "127.0.0.1"
	}
	return host, port, nil
}

// checkPortFuser проверяет порт через fuser (возвращает PID процесса).
func checkPortFuser(port string) (bool, string) {
	cmd := exec.Command("fuser", port+"/tcp", "2>/dev/null")
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}
	outStr := strings.TrimSpace(string(output))
	if outStr == "" {
		return false, ""
	}

	// fuser выводит PID через пробел: "1234 5678"
	pids := strings.Fields(outStr)
	if len(pids) == 0 {
		return false, ""
	}

	// Пробуем получить имя процесса по PID
	var infoLines []string
	for _, pid := range pids {
		pid = strings.TrimSuffix(pid, "/tcp")
		procName := getProcessName(pid)
		if procName != "" {
			infoLines = append(infoLines, fmt.Sprintf("PID %s: %s", pid, procName))
		} else {
			infoLines = append(infoLines, fmt.Sprintf("PID %s", pid))
		}
	}
	return true, strings.Join(infoLines, "\n")
}

// getProcessName возвращает имя процесса по PID
func getProcessName(pid string) string {
	cmd := exec.Command("ps", "-p", pid, "-o", "comm=")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// checkPortSS проверяет порт через ss.
func checkPortSS(port string) (bool, string) {
	cmd := exec.Command("ss", "-tlnp", fmt.Sprintf("sport = :%s", port))
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return false, ""
	}
	// Первая строка — заголовок, остальные — данные
	return true, strings.Join(lines, "\n")
}

// checkPortLsof проверяет порт через lsof.
func checkPortLsof(port string) (bool, string) {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("lsof -i:%s -sTCP:LISTEN -P -n 2>/dev/null", port))
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}
	outStr := strings.TrimSpace(string(output))
	if outStr == "" {
		return false, ""
	}
	return true, outStr
}

// runNmap запускает nmap -sV на указанном порту и выводит результат.
func runNmap(host, port string) {
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		fmt.Printf("invalid port: %s\n", port)
		return
	}

	cmd := exec.Command("nmap", "-sV", "-T4", "-p", port, host)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("nmap: %v\n", err)
	}
}
