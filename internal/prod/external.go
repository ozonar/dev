package prod

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// externalDepsPath — путь к файлу со списком внешних зависимостей.
const externalDepsPath = "/etc/prod-command/deps.conf"

// externalDep описывает одну внешнюю зависимость.
type externalDep struct {
	Name string
	URL  string
}

// loadExternalDeps читает зависимости из конфига (name=url построчно).
func loadExternalDeps() []externalDep {
	f, err := os.Open(externalDepsPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	var deps []externalDep
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		deps = append(deps, externalDep{Name: strings.TrimSpace(parts[0]), URL: strings.TrimSpace(parts[1])})
	}
	return deps
}

// measureLatency измеряет задержку HTTP-запроса в миллисекундах.
func measureLatency(url string) (ms float64, ok bool) {
	start := time.Now()
	code := execOut("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "3", "-k", url)
	ms = float64(time.Since(start)) / float64(time.Millisecond)
	if code == "" {
		return ms, false
	}
	return ms, true
}

// collectExternal собирает категорию External dependencies.
func collectExternal(prev *Report) *Category {
	cat := &Category{ID: CatExternal, Present: true}
	deps := loadExternalDeps()

	// Если зависимости не сконфигурированы (deps.conf пуст/отсутствует) —
	// скрываем категорию полностью, чтобы не мусорить отчёт.
	if len(deps) == 0 {
		cat.Present = false
		return cat
	}
	cat.Data = fmt.Sprintf("deps=%d", len(deps))
	cat.Detected = true

	for _, dep := range deps {
		ms, ok := measureLatency(dep.URL)
		if !ok {
			cat.AddSymptom(Symptom{ID: "EXTERNAL_DEPENDENCY_CONNECTION_ERROR", Level: LevelWarn,
				ProcessName: dep.Name,
				Summary:     fmt.Sprintf("%s — connection failed", dep.Name)})
			continue
		}
		switch {
		case ms >= 1000:
			cat.AddSymptom(Symptom{ID: "EXTERNAL_DEPENDENCY_LATENCY", Level: LevelError,
				ProcessName: dep.Name,
				Summary:     fmt.Sprintf("%s latency elevated — %dms", dep.Name, int(ms))})
		case ms >= 300:
			cat.AddSymptom(Symptom{ID: "EXTERNAL_DEPENDENCY_LATENCY", Level: LevelWarn,
				ProcessName: dep.Name,
				Summary:     fmt.Sprintf("%s latency elevated — %dms", dep.Name, int(ms))})
		default:
			cat.AddSymptom(Symptom{ID: "EXTERNAL_DEPENDENCY_LATENCY", Level: LevelOK,
				ProcessName: dep.Name,
				Summary:     fmt.Sprintf("%s — %dms", dep.Name, int(ms))})
		}
	}

	return cat
}
