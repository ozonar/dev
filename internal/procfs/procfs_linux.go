//go:build linux

package procfs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// tcpTables — таблицы TCP-сокетов IPv4 и IPv6.
var tcpTables = []string{"/proc/net/tcp", "/proc/net/tcp6"}

// tcpStateListen — код состояния LISTEN в таблицах TCP.
const tcpStateListen = "0A"

// procDir — корень файловой системы процессов.
const procDir = "/proc"

// Listeners возвращает владельцев слушающих сокетов на порту.
// Пустой срез означает, что порт свободен.
func Listeners(port int) ([]Listener, error) {
	inodes, err := listenInodes(port)
	if err != nil {
		return nil, err
	}

	var out []Listener
	for _, inode := range inodes {
		pids, err := socketOwners(inode)
		if err != nil {
			return nil, err
		}
		if len(pids) == 0 {
			out = append(out, Listener{})
			continue
		}
		for _, pid := range pids {
			out = append(out, Listener{PID: pid, Name: processName(pid)})
		}
	}
	return out, nil
}

// Holders возвращает PID процессов, удерживающих файл path: открытых через
// файловые дескрипторы или отображённых в память (mmap).
func Holders(path string) ([]int, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %v", path, err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}

	byFD, err := scanProcFD(func(link string) bool { return link == abs })
	if err != nil {
		return nil, err
	}
	byMaps, err := scanProcMaps(abs)
	if err != nil {
		return nil, err
	}
	return mergePIDs(byFD, byMaps), nil
}

// listenInodes собирает inode слушающих сокетов на порту.
// Отсутствие /proc/net/tcp6 не считается ошибкой: IPv6 может быть отключён.
func listenInodes(port int) ([]string, error) {
	opened := 0
	seen := make(map[string]bool)
	var inodes []string

	for _, table := range tcpTables {
		f, err := os.Open(table)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to open %s: %v", table, err)
		}
		opened++

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			p, state, inode, ok := parseTCPLine(sc.Text())
			if !ok || p != port || state != tcpStateListen {
				continue
			}
			if inode == "" || inode == "0" || seen[inode] {
				continue
			}
			seen[inode] = true
			inodes = append(inodes, inode)
		}
		scanErr := sc.Err()
		f.Close()
		if scanErr != nil {
			return nil, fmt.Errorf("failed to read %s: %v", table, scanErr)
		}
	}

	if opened == 0 {
		return nil, fmt.Errorf("no TCP tables available in %s/net", procDir)
	}
	return inodes, nil
}

// socketOwners возвращает PID процессов, у которых открыт сокет с inode.
func socketOwners(inode string) ([]int, error) {
	target := "socket:[" + inode + "]"
	return scanProcFD(func(link string) bool { return link == target })
}

// scanProcFD обходит дескрипторы всех процессов и возвращает PID, у которых
// есть ссылка, удовлетворяющая match. Нечитаемые процессы пропускаются:
// они либо завершились, либо недоступны без прав — это не ошибка детекции.
func scanProcFD(match func(link string) bool) ([]int, error) {
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", procDir, err)
	}

	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join(procDir, e.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if match(link) {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids, nil
}

// scanProcMaps обходит карты памяти всех процессов и возвращает PID,
// у которых файл path отображён в память.
func scanProcMaps(path string) ([]int, error) {
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", procDir, err)
	}

	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		f, err := os.Open(filepath.Join(procDir, e.Name(), "maps"))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		found := false
		for sc.Scan() {
			if mapsHoldsPath(sc.Text(), path) {
				found = true
				break
			}
		}
		f.Close()
		if found {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// processName возвращает имя процесса по PID или пустую строку.
func processName(pid int) string {
	b, err := os.ReadFile(filepath.Join(procDir, strconv.Itoa(pid), "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// mergePIDs объединяет два списка PID без дубликатов в отсортированный срез.
func mergePIDs(lists ...[]int) []int {
	seen := make(map[int]bool)
	var out []int
	for _, list := range lists {
		for _, pid := range list {
			if seen[pid] {
				continue
			}
			seen[pid] = true
			out = append(out, pid)
		}
	}
	sort.Ints(out)
	return out
}

// parseTCPLine разбирает строку таблицы TCP: локальный порт, состояние
// и inode сокета. ok=false для заголовка и строк с недостатком полей.
func parseTCPLine(line string) (port int, state, inode string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 10 {
		return 0, "", "", false
	}

	local := fields[1]
	idx := strings.LastIndex(local, ":")
	if idx < 0 {
		return 0, "", "", false
	}
	p, err := strconv.ParseInt(local[idx+1:], 16, 32)
	if err != nil {
		return 0, "", "", false
	}
	return int(p), fields[3], fields[9], true
}

// mapsHoldsPath проверяет, ссылается ли строка /proc/<pid>/maps на путь path.
// Формат строки: address perms offset dev inode pathname. У pathname может
// быть суффикс " (deleted)" и пробелы в самом пути.
func mapsHoldsPath(line, path string) bool {
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return false
	}
	name := strings.Join(fields[5:], " ")
	name = strings.TrimSuffix(name, " (deleted)")
	return name == path
}
