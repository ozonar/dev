//go:build !linux && !windows

package procfs

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// lsofPath — системная утилита, используемая на платформах без /proc.
// На macOS и BSD lsof присутствует по умолчанию.
const lsofPath = "lsof"

// Listeners возвращает владельцев слушающих сокетов на порту через lsof.
func Listeners(port int) ([]Listener, error) {
	args := []string{"-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-Fpcn"}
	out, err := exec.Command(lsofPath, args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		// lsof завершается с кодом 1, когда совпадений нет.
		if errors.As(err, &exitErr) && len(out) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("lsof failed: %v", err)
	}
	return parseLsofListeners(out), nil
}

// Holders возвращает PID процессов, открывших файл path, через lsof.
func Holders(path string) ([]int, error) {
	out, err := exec.Command(lsofPath, "-t", "--", path).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(out) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("lsof failed: %v", err)
	}

	var pids []int
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		pid, err := strconv.Atoi(strings.TrimSpace(sc.Text()))
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}
