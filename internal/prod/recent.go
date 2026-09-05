package prod

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// deployMarkers — пути, изменение которых может указывать на деплой.
var deployMarkers = []string{
	".git/HEAD",
	".git/refs/heads/main",
	"deploy.log",
	"release",
	"VERSION",
}

// collectRecent собирает категорию Recent changes (корреляционные факты).
func collectRecent(prev *Report) *Category {
	cat := &Category{ID: CatRecent, Present: true}

	// RECENT_DEPLOY: недавнее изменение маркеров деплоя.
	if ago, ok := recentDeploy(); ok {
		if ago < 30*time.Minute {
			cat.AddSymptom(Symptom{ID: "RECENT_DEPLOY", Level: LevelWarn,
				Summary: fmt.Sprintf("deploy detected %s ago", humanAgo(ago))})
		}
	}

	// PROCESS_RESTART: сравнение времени старта ключевых процессов с прошлым
	// снапшотом.
	if prev != nil {
		if pc := prev.Category(CatRecent); pc != nil {
			// Фиксируем стартовое время php-fpm и nginx мастер-процессов.
			curStarts := keyProcessStarts()
			for name, st := range curStarts {
				for _, s := range pc.Symptoms {
					if s.ID == "PROCESS_START_TIME" && s.ProcessName == name {
						if prevTime := int64(s.Value); prevTime > 0 && st-prevTime < 600 {
							cat.AddSymptom(Symptom{ID: "PROCESS_RESTART", Level: LevelWarn,
								ProcessName: name,
								Summary:     fmt.Sprintf("%s restarted recently", name)})
						}
					}
				}
			}
			// Сохраняем текущие стартовые времена для следующего снапшота.
			for name, st := range curStarts {
				cat.AddSymptom(Symptom{ID: "PROCESS_START_TIME", Level: LevelOK,
					ProcessName: name,
					Value:       float64(st)})
			}
		}
	}

	cat.Detected = true
	return cat
}

// recentDeploy возвращает время с последнего изменения маркера деплоя.
func recentDeploy() (time.Duration, bool) {
	now := time.Now()
	for _, path := range deployMarkers {
		if fi, err := os.Stat(path); err == nil {
			ago := now.Sub(fi.ModTime())
			if ago < 12*time.Hour {
				return ago, true
			}
		}
	}
	return 0, false
}

// keyProcessStarts возвращает стартовое время (в сек от boot) ключевых служб.
func keyProcessStarts() map[string]int64 {
	res := make(map[string]int64)
	for _, p := range processInfos() {
		name := strings.ToLower(p.Name)
		switch {
		case strings.HasPrefix(name, "php-fpm: master"):
			res["php-fpm"] = processStartSeconds(p.PID)
		case strings.HasPrefix(name, "nginx: master"):
			res["nginx"] = processStartSeconds(p.PID)
		case strings.Contains(name, "postgres") && strings.Contains(name, "postmaster"):
			res["postgres"] = processStartSeconds(p.PID)
		}
	}
	return res
}

// processStartSeconds возвращает время старта процесса (сек от boot).
func processStartSeconds(pid int) int64 {
	s, err := readFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	closeIdx := strings.LastIndex(s, ")")
	if closeIdx < 0 {
		return 0
	}
	rest := s[closeIdx+1:]
	f := strings.Fields(rest)
	// Поле 22 = starttime -> индекс 19 (после state=3).
	if len(f) < 22 {
		return 0
	}
	return parseInt(f[19]) / 100 // грубо: starttime в jiffies, делим на HZ~100
}

// humanAgo форматирует длительность как "N min" / "N sec" / "N h".
func humanAgo(d time.Duration) string {
	mins := int(d.Minutes())
	if mins >= 60 {
		return fmt.Sprintf("%dh %dm", mins/60, mins%60)
	}
	if mins >= 1 {
		return fmt.Sprintf("%d min", mins)
	}
	return fmt.Sprintf("%d sec", int(d.Seconds()))
}
