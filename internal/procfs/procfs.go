// Пакет procfs предоставляет нативную работу с процессами и сокетами:
// поиск владельцев слушающего порта, поиск процессов, удерживающих файл,
// и завершение найденных процессов.
//
// На Linux данные берутся из /proc. На платформах без /proc используется
// системный lsof, который там присутствует по умолчанию.
package procfs

import (
	"fmt"
	"os"
)

// Listener — процесс, слушающий TCP-порт. PID == 0 означает, что сокет
// найден, но его владелец не определён (нет прав или сокет без процесса).
type Listener struct {
	PID  int
	Name string
}

// KillPID завершает процесс принудительно.
func KillPID(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %v", pid, err)
	}
	if err := p.Kill(); err != nil {
		return fmt.Errorf("failed to kill process %d: %v", pid, err)
	}
	return nil
}

// KillPort завершает процессы, слушающие порт.
// Если порт занят, но хотя бы один владелец не определён, возвращается
// ошибка: состояние не должно трактоваться как «убивать нечего».
func KillPort(port int) error {
	listeners, err := Listeners(port)
	if err != nil {
		return err
	}
	unknown := false
	for _, l := range listeners {
		if l.PID == 0 {
			unknown = true
			continue
		}
		if err := KillPID(l.PID); err != nil {
			return err
		}
	}
	if unknown {
		return fmt.Errorf("port %d is in use but some owners cannot be determined", port)
	}
	return nil
}

// KillFile завершает процессы, удерживающие файл path (через fd или mmap).
// Текущий процесс не завершается. Если файл никто не удерживает — не ошибка.
func KillFile(path string) error {
	pids, err := Holders(path)
	if err != nil {
		return err
	}
	self := os.Getpid()
	for _, pid := range pids {
		if pid == self {
			continue
		}
		if err := KillPID(pid); err != nil {
			return err
		}
	}
	return nil
}
