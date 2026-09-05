package prod

import (
	"os/exec"
	"strings"
)

// tailJournal возвращает последние n строк журнала. Источник выбирается в
// порядке приоритета: journalctl --since, dmesg. При недоступности обоих —
// пустой срез.
func tailJournal(unit string, n int) []string {
	// Пытаемся использовать journalctl (systemd).
	if lines, ok := journalctlLines(unit, n); ok {
		return lines
	}
	// Падение на dmesg для kernel-сообщений.
	if unit == "kernel" {
		if lines, ok := dmesgLines(n); ok {
			return lines
		}
	}
	return nil
}

// journalctlLines запрашивает строки из systemd journal.
func journalctlLines(unit string, n int) ([]string, bool) {
	if !procExists("/run/systemd/system") {
		return nil, false
	}
	args := []string{"-n", "20", "--no-pager"}
	if unit == "kernel" {
		args = append(args, "-k")
	} else {
		args = append(args, "-u", unit)
	}
	cmd := exec.Command("journalctl", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines, true
}

// dmesgLines запрашивает последние строки кольцевого буфера ядра.
func dmesgLines(n int) ([]string, bool) {
	cmd := exec.Command("dmesg")
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, true
}
