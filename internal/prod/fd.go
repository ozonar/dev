package prod

import (
	"fmt"
	"sort"
	"strings"
)

// systemFDInfo возвращает использование системных файловых дескрипторов.
func systemFDInfo() (used, max int64) {
	// /proc/sys/fs/file-nr: уже открыто, свободно, максимум.
	s, err := readFile("/proc/sys/fs/file-nr")
	if err != nil {
		return 0, 0
	}
	var open, free, limit int64
	fmt.Sscanf(s, "%d %d %d", &open, &free, &limit)
	return open, limit
}

// collectFD собирает категорию File descriptors.
func collectFD(prev *Report) *Category {
	cat := &Category{ID: CatFD, Present: true}

	used, max := systemFDInfo()
	if used == 0 && max == 0 {
		cat.Present = false
		return cat
	}
	pct := float64(used) / float64(max) * 100
	cat.Data = fmt.Sprintf("used=%d max=%d pct=%.1f", used, max, pct)

	// SYSTEM_FILE_DESCRIPTOR_EXHAUSTION.
	switch {
	case pct >= 90:
		cat.AddSymptom(Symptom{ID: "SYSTEM_FILE_DESCRIPTOR_EXHAUSTION", Level: LevelError,
			Summary: fmt.Sprintf("system: %d / %d (%.0f%%)", used, max, pct)})
	case pct >= 75:
		cat.AddSymptom(Symptom{ID: "SYSTEM_FILE_DESCRIPTOR_EXHAUSTION", Level: LevelWarn,
			Summary: fmt.Sprintf("system: %d / %d (%.0f%%)", used, max, pct)})
	default:
		cat.AddSymptom(Symptom{ID: "SYSTEM_FILE_DESCRIPTOR_EXHAUSTION", Level: LevelOK,
			Summary: fmt.Sprintf("system: %d / %d", used, max)})
	}

	// FILE_DESCRIPTOR_EXHAUSTION: топ процессов по числу fd с учётом их лимита.
	type fdProc struct {
		name string
		pid  int
		used int64
		soft int64
		pct  float64
	}
	var procs []fdProc
	for _, p := range processInfos() {
		if p.Name == "" || p.FDCount == 0 {
			continue
		}
		soft := fdLimit(p.PID)
		pct := 0.0
		if soft > 0 {
			pct = float64(p.FDCount) / float64(soft) * 100
		}
		procs = append(procs, fdProc{name: p.Name, pid: p.PID, used: int64(p.FDCount), soft: soft, pct: pct})
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].pct > procs[j].pct })
	added := 0
	for _, p := range procs {
		if p.pct < 85 && p.used < 10000 {
			continue
		}
		lvl := LevelWarn
		if p.pct >= 95 || (p.soft > 0 && p.used >= p.soft) {
			lvl = LevelError
		}
		cat.AddSymptom(Symptom{
			ID:          "FILE_DESCRIPTOR_EXHAUSTION",
			Level:       lvl,
			ProcessName: p.name,
			Summary:     fmt.Sprintf("%s: %d / %d (%.0f%%)", p.name, p.used, p.soft, p.pct),
			Detail:      fmt.Sprintf("PID %d holds %d file descriptors", p.pid, p.used),
			Value:       float64(p.used),
		})
		added++
		if added >= 5 {
			break
		}
	}

	// FD_LEAK: рост числа fd у процесса относительно прошлого снапшота.
	if prev != nil {
		if pc := prev.Category(CatFD); pc != nil {
			cur := make(map[string]int64)
			for _, s := range cat.Symptoms {
				if s.ID == "FILE_DESCRIPTOR_EXHAUSTION" {
					cur[s.ProcessName] = int64(s.Value)
				}
			}
			for _, s := range pc.Symptoms {
				if s.ID != "FILE_DESCRIPTOR_EXHAUSTION" {
					continue
				}
				prevN := int64(s.Value)
				if curN, ok := cur[s.ProcessName]; ok && prevN > 0 && curN > prevN*3 {
					cat.AddSymptom(Symptom{ID: "FD_LEAK", Level: LevelWarn, ProcessName: s.ProcessName,
						Summary: fmt.Sprintf("%s: %d -> %d fds (possible leak)", s.ProcessName, prevN, curN)})
				}
			}
		}
	}

	cat.Detected = true
	return cat
}

// fdLimit возвращает мягкий лимит числа fd для процесса.
func fdLimit(pid int) int64 {
	path := fmt.Sprintf("/proc/%d/limits", pid)
	s, err := readFile(path)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "Max open files") {
			f := strings.Fields(line)
			if len(f) >= 4 {
				if f[1] == "unlimited" {
					return 0 // неизвестный предел — не оцениваем процент
				}
				return parseInt(f[1])
			}
		}
	}
	return 0
}
