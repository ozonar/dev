package procfs

import (
	"bufio"
	"bytes"
	"strconv"
)

// parseLsofListeners разбирает вывод lsof -Fpcn: строки p<pid> и c<command>.
// Несколько строк n на один процесс схлопываются в одного владельца.
func parseLsofListeners(out []byte) []Listener {
	var listeners []Listener
	seen := make(map[int]bool)
	var pid int
	var name string

	appendCurrent := func() {
		if pid == 0 || seen[pid] {
			return
		}
		seen[pid] = true
		listeners = append(listeners, Listener{PID: pid, Name: name})
	}

	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			appendCurrent()
			pid, _ = strconv.Atoi(line[1:])
			name = ""
		case 'c':
			name = line[1:]
		}
	}
	appendCurrent()
	return listeners
}
