package prod

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"dev/internal/colors"
	"dev/internal/logs"
)

// logMenuItem — пункт верхнего уровня меню логов.
type logMenuItem struct {
	label string
	kind  string // "folder", "user", "core"
	svc   *ServiceInfo
}

// RunLogs показывает интерактивное меню источников логов: текущая папка
// (дефолт 1), пользовательские и core службы. После выбора открывает
// под-список логов или сам источник.
func RunLogs() {
	cwd, _ := os.Getwd()
	rep := CollectServices(6 * time.Hour)
	// Единый reader на всё меню: отдельные reader'ы теряют забуференный ввод.
	reader := bufio.NewReader(os.Stdin)

	// Пункты меню: текущая папка, затем пользовательские, затем core службы.
	items := []logMenuItem{{label: "Current folder (" + cwd + ")", kind: "folder"}}
	for i := range rep.Services {
		s := &rep.Services[i]
		items = append(items, logMenuItem{label: ServiceLabel(s), kind: "user", svc: s})
	}
	for i := range rep.Core {
		s := &rep.Core[i]
		items = append(items, logMenuItem{label: ServiceLabel(s), kind: "core", svc: s})
	}

	fmt.Println(colors.Cyan("Available logs:"))
	prevKind := ""
	for i, it := range items {
		if it.kind != prevKind {
			switch it.kind {
			case "user":
				fmt.Println("  " + colors.Gray("User services:"))
			case "core":
				fmt.Println("  " + colors.Gray("Core services:"))
			}
			prevKind = it.kind
		}
		fmt.Printf("  %d) %s\n", i+1, it.label)
	}

	fmt.Print(colors.Cyan("Select [1]: "))
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	idx := 1
	if line != "" {
		n, err := strconv.Atoi(line)
		if err != nil || n < 1 || n > len(items) {
			fmt.Println(colors.Red("Invalid selection"))
			return
		}
		idx = n
	}

	sel := items[idx-1]
	switch sel.kind {
	case "folder":
		openFolderLogs(cwd, reader)
	default:
		openServiceLogs(*sel.svc, reader)
	}
}

// openFolderLogs ищет логи в текущей папке (через dev logs) и открывает
// выбранный источник.
func openFolderLogs(cwd string, reader *bufio.Reader) {
	entries, err := logs.FindLogs(cwd)
	if err != nil {
		fmt.Println(colors.Red("Error finding logs: " + err.Error()))
		return
	}
	if len(entries) == 0 {
		fmt.Println(colors.Yellow("No log files or docker containers found."))
		return
	}
	fmt.Println(colors.Cyan("Available logs in " + cwd + ":"))
	for i, e := range entries {
		fmt.Printf("  %d) %s\n", i+1, logEntryLabel(e))
	}
	fmt.Print(colors.Cyan("Select (or 0 to exit): "))
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" || line == "0" {
		return
	}
	idx, err := strconv.Atoi(line)
	if err != nil || idx < 1 || idx > len(entries) {
		fmt.Println(colors.Red("Invalid selection"))
		return
	}
	openLogEntry(entries[idx-1])
}

// openServiceLogs показывает под-список логов службы: journalctl (дефолт 1)
// плюс найденные файлы. Если файлов нет — открывает journal сразу.
func openServiceLogs(s ServiceInfo, reader *bufio.Reader) {
	entries := ServiceLogSources(s)
	if len(entries) <= 1 {
		openLogEntry(entries[0])
		return
	}
	fmt.Println(colors.Cyan("Available logs for " + s.Name + ":"))
	for i, e := range entries {
		fmt.Printf("  %d) %s\n", i+1, logEntryLabel(e))
	}
	fmt.Print(colors.Cyan("Select [1]: "))
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	idx := 1
	if line != "" {
		n, err := strconv.Atoi(line)
		if err != nil || n < 1 || n > len(entries) {
			fmt.Println(colors.Red("Invalid selection"))
			return
		}
		idx = n
	}
	openLogEntry(entries[idx-1])
}

// logEntryLabel возвращает подпись источника логов для меню.
func logEntryLabel(e logs.LogEntry) string {
	switch e.Type {
	case "journal":
		return colors.Cyan("journalctl")
	case "docker":
		return colors.Yellow("[docker] " + e.Path)
	default:
		return colors.White(e.Path)
	}
}

// openLogEntry открывает выбранный источник логов.
func openLogEntry(e logs.LogEntry) {
	fmt.Println(colors.Green("Opening " + e.Path + " (" + e.Type + ")..."))
	if err := OpenLog(e); err != nil {
		fmt.Println(colors.Red("Failed to open log: " + err.Error()))
	}
}
