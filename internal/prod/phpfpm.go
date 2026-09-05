package prod

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fpmState — снимок состояния php-fpm.
type fpmState struct {
	Present     bool
	Workers     int
	MaxChildren int
	MasterPID   int
	ConfigPath  string
}

// detectFPM находит php-fpm master и считает воркеров.
func detectFPM() fpmState {
	var st fpmState
	procs := processInfos()
	for _, p := range procs {
		name := strings.ToLower(p.Name)
		if strings.Contains(name, "php-fpm") || strings.Contains(name, "php-fpm:") {
			st.Present = true
			// master-процесс имеет comm "php-fpm: master process (...)".
			if strings.Contains(name, "master") {
				st.MasterPID = p.PID
			}
		}
	}
	if !st.Present {
		return st
	}
	// Считаем воркеров через ps (comm у воркеров содержит "pool").
	out := execOut("sh", "-c", "ps -eo comm= | grep -c 'php-fpm: pool' || true")
	st.Workers = int(parseInt(out))
	// Если ps недоступен — приблизительная оценка по процессам, не являющимся master.
	if st.Workers == 0 {
		for _, p := range procs {
			if strings.Contains(strings.ToLower(p.Name), "php-fpm") && p.PID != st.MasterPID {
				st.Workers++
			}
		}
	}

	// Максимум воркеров из конфига пула.
	st.ConfigPath = discoverFPMConfig()
	st.MaxChildren = fpmMaxChildren(st.ConfigPath)
	return st
}

// discoverFPMConfig ищет файл конфигурации php-fpm pool.
func discoverFPMConfig() string {
	candidates := []string{
		"/etc/php/8.3/fpm/pool.d/www.conf",
		"/etc/php/8.2/fpm/pool.d/www.conf",
		"/etc/php/8.1/fpm/pool.d/www.conf",
		"/etc/php/7.4/fpm/pool.d/www.conf",
		"/etc/php-fpm.d/www.conf",
		"/usr/local/etc/php-fpm.d/www.conf",
	}
	// Поиск по glob для учёта прочих версий.
	matches, _ := filepath.Glob("/etc/php/*/fpm/pool.d/*.conf")
	for _, m := range matches {
		candidates = append(candidates, m)
	}
	for _, c := range candidates {
		if procExists(c) {
			return c
		}
	}
	return ""
}

// fpmMaxChildren извлекает pm.max_children из конфига пула.
func fpmMaxChildren(path string) int {
	if path == "" || !procExists(path) {
		return 0
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pm.max_children") {
			f := strings.Fields(line)
			if len(f) >= 3 {
				return int(parseInt(f[2]))
			}
		}
	}
	return 0
}

// collectPHPFPM собирает категорию PHP-FPM.
func collectPHPFPM(prev *Report) *Category {
	cat := &Category{ID: CatPHPFPM, Present: true}
	st := detectFPM()
	if !st.Present {
		// PHP-FPM не обнаружен — категория не показывается.
		cat.Present = false
		return cat
	}

	cat.Data = fmt.Sprintf("workers=%d max_children=%d", st.Workers, st.MaxChildren)

	// FPM_WORKER_SATURATION.
	switch {
	case st.MaxChildren > 0 && st.Workers >= st.MaxChildren:
		cat.AddSymptom(Symptom{ID: "FPM_WORKER_SATURATION", Level: LevelError,
			Summary: fmt.Sprintf("%d/%d workers busy", st.Workers, st.MaxChildren)})
	case st.MaxChildren > 0 && float64(st.Workers)/float64(st.MaxChildren) >= 0.8:
		cat.AddSymptom(Symptom{ID: "FPM_WORKER_SATURATION", Level: LevelWarn,
			Summary: fmt.Sprintf("%d/%d workers busy", st.Workers, st.MaxChildren)})
	case st.MaxChildren > 0:
		cat.AddSymptom(Symptom{ID: "FPM_WORKER_SATURATION", Level: LevelOK,
			Summary: fmt.Sprintf("%d/%d workers busy", st.Workers, st.MaxChildren)})
	default:
		cat.AddSymptom(Symptom{ID: "FPM_WORKER_SATURATION", Level: LevelOK,
			Summary: fmt.Sprintf("%d workers active", st.Workers)})
	}

	// FPM_LISTEN_QUEUE: число ожидающих в accept-очереди сокетов php-fpm.
	if q, ok := fpmListenQueue(); ok && q > 0 {
		lvl := LevelWarn
		if q > 100 {
			lvl = LevelError
		}
		cat.AddSymptom(Symptom{ID: "FPM_LISTEN_QUEUE", Level: lvl,
			Summary: fmt.Sprintf("%d requests waiting in listen queue", q)})
	}

	// FPM_SLOW_REQUESTS: наличие сообщений о медленных запросах в логах.
	if n, ok := fpmSlowRequests(); ok && n > 0 {
		cat.AddSymptom(Symptom{ID: "FPM_SLOW_REQUESTS", Level: LevelWarn,
			Summary: fmt.Sprintf("%d slow requests in recent log", n)})
	}

	// FPM_WORKER_CRASH: признаки падения воркеров в журнале.
	if n, ok := fpmWorkerCrashes(); ok && n > 0 {
		cat.AddSymptom(Symptom{ID: "FPM_WORKER_CRASH", Level: LevelError,
			Summary: fmt.Sprintf("%d worker crashes detected", n)})
	}

	cat.Detected = true
	return cat
}

// fpmListenQueue оценивает длину accept-очереди для слушающего сокета php-fpm.
func fpmListenQueue() (int64, bool) {
	// Порты php-fpm обычно 9000/9001; смотрим Recv-Q у LISTEN на этих портах.
	var total int64
	found := false
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		s, err := readFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, "sl") {
				continue
			}
			f := strings.Fields(line)
			if len(f) < 5 {
				continue
			}
			// f[1] — локальный адрес ip:port в hex.
			addrParts := strings.Split(f[1], ":")
			if len(addrParts) != 2 {
				continue
			}
			port := parseIntHex(addrParts[1])
			if port != 9000 && port != 9001 {
				continue
			}
			if !strings.Contains(strings.ToUpper(f[4]), "0A") { // LISTEN
				continue
			}
			rx := strings.Split(f[3], ":")[0]
			v := parseIntHex(rx)
			total += v
			found = true
		}
	}
	return total, found
}

// parseIntHex преобразует hex-строку в int64.
func parseIntHex(s string) int64 {
	var n int64
	for _, c := range strings.TrimSpace(s) {
		n = n*16 + int64(hexVal(c))
	}
	return n
}

func hexVal(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return 0
	}
}

// fpmSlowRequests ищет slow request логи в журнале php-fpm.
func fpmSlowRequests() (int, bool) {
	count := 0
	for _, unit := range []string{"php8.3-fpm", "php8.2-fpm", "php8.1-fpm", "php7.4-fpm", "php-fpm"} {
		for _, l := range tailJournal(unit, 60) {
			if strings.Contains(strings.ToLower(l), "slow") && strings.Contains(strings.ToLower(l), "request") {
				count++
			}
		}
	}
	return count, count > 0
}

// fpmWorkerCrashes ищет признаки падения воркеров.
func fpmWorkerCrashes() (int, bool) {
	count := 0
	for _, unit := range []string{"php8.3-fpm", "php8.2-fpm", "php8.1-fpm", "php7.4-fpm", "php-fpm"} {
		for _, l := range tailJournal(unit, 60) {
			low := strings.ToLower(l)
			if strings.Contains(low, "seems to be crashed") ||
				strings.Contains(low, "reached max_children") ||
				strings.Contains(low, "exited on signal") {
				count++
			}
		}
	}
	return count, count > 0
}
