package prod

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// readFile читает файл целиком, возвращая ошибку в случае недоступности.
func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// procExists проверяет существование файла/каталога.
func procExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// parseInt безопасно преобразует строку в int64.
func parseInt(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// parseFloat безопасно преобразует строку в float64.
func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

// pidList возвращает список числовых PID из /proc.
func pidList() []int {
	var pids []int
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		n, err := strconv.Atoi(e.Name())
		if err == nil {
			pids = append(pids, n)
		}
	}
	sort.Ints(pids)
	return pids
}

// processInfo хранит агрегированную информацию о процессе.
type processInfo struct {
	PID      int
	Name     string // comm
	User     string
	CPU      float64 // использованный CPU, сек (jiffies)
	RSS      int64   // resident set size, байт
	FDCount  int
	OOMScore int
}

// processInfos собирает информацию обо всех процессах.
func processInfos() []processInfo {
	var out []processInfo
	for _, pid := range pidList() {
		dir := fmt.Sprintf("/proc/%d", pid)
		if !procExists(dir) {
			continue
		}
		pi := processInfo{PID: pid}
		// comm
		if c, err := readFile(dir + "/comm"); err == nil {
			pi.Name = strings.TrimSpace(c)
		}
		// stat: поле 14 utime, 15 stime (1-indexed через скобки имени)
		if s, err := readFile(dir + "/stat"); err == nil {
			pi.CPU = parseStatCPU(s)
		}
		// status: RSS и Uid
		if s, err := readFile(dir + "/status"); err == nil {
			sc := bufio.NewScanner(strings.NewReader(s))
			for sc.Scan() {
				line := sc.Text()
				switch {
				case strings.HasPrefix(line, "RssAnon:"):
					// kB -> bytes
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						pi.RSS = parseInt(fields[1]) * 1024
					}
				case strings.HasPrefix(line, "VmRSS:"):
					fields := strings.Fields(line)
					if len(fields) >= 2 && pi.RSS == 0 {
						pi.RSS = parseInt(fields[1]) * 1024
					}
				case strings.HasPrefix(line, "Uid:"):
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						pi.User = fields[1]
					}
				}
			}
		}
		// fd count
		if fd, err := os.ReadDir(dir + "/fd"); err == nil {
			pi.FDCount = len(fd)
		}
		// oom_score
		if o, err := readFile(dir + "/oom_score"); err == nil {
			pi.OOMScore = int(parseInt(o))
		}
		out = append(out, pi)
	}
	return out
}

// parseStatCPU извлекает utime+stime из /proc/<pid>/stat.
// Учитывает возможные пробелы в имени процесса (в скобках).
func parseStatCPU(stat string) float64 {
	// имя процесса в скобках; после закрывающей скобки начинаются поля с 3-го
	closeIdx := strings.LastIndex(stat, ")")
	if closeIdx < 0 {
		return 0
	}
	rest := strings.TrimSpace(stat[closeIdx+1:])
	fields := strings.Fields(rest)
	// rest начинается с поля 3 (state). Поле 14 = state(3) -> индекс 11,
	// поле 15 = индекс 12.
	if len(fields) < 15 {
		return 0
	}
	utime := parseFloat(fields[11])
	stime := parseFloat(fields[12])
	return utime + stime
}

// loadAvg возвращает три значения load average.
func loadAvg() [3]float64 {
	var out [3]float64
	s, err := readFile("/proc/loadavg")
	if err != nil {
		return out
	}
	fields := strings.Fields(s)
	for i := 0; i < 3 && i < len(fields); i++ {
		out[i] = parseFloat(fields[i])
	}
	return out
}

// execOut выполняет команду и возвращает stdout. Ошибки не критичны —
// возвращается пустая строка.
func execOut(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// cpuCount возвращает число ядер CPU.
func cpuCount() int {
	s, err := readFile("/proc/cpuinfo")
	if err != nil {
		return 0
	}
	count := 0
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "processor") {
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return count
}
