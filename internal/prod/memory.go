package prod

import (
	"fmt"
	"sort"
	"strings"
)

// memInfo — снимок /proc/meminfo.
type memInfo struct {
	MemTotal     int64
	MemAvailable int64
	MemFree      int64
	Buffers      int64
	Cached       int64
	SwapTotal    int64
	SwapFree     int64
	SwapCached   int64
}

// readMemInfo парсит /proc/meminfo (значения в kB).
func readMemInfo() memInfo {
	var m memInfo
	s, err := readFile("/proc/meminfo")
	if err != nil {
		return m
	}
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		val := parseInt(f[1]) // kB
		switch f[0] {
		case "MemTotal:":
			m.MemTotal = val
		case "MemAvailable:":
			m.MemAvailable = val
		case "MemFree:":
			m.MemFree = val
		case "Buffers:":
			m.Buffers = val
		case "Cached:":
			m.Cached = val
		case "SwapTotal:":
			m.SwapTotal = val
		case "SwapFree:":
			m.SwapFree = val
		case "SwapCached:":
			m.SwapCached = val
		}
	}
	return m
}

// collectMemory собирает категорию Memory.
func collectMemory(prev *Report) *Category {
	cat := &Category{ID: CatMemory, Present: true}
	m := readMemInfo()

	if m.MemTotal == 0 {
		cat.Present = false
		return cat
	}

	used := m.MemTotal - m.MemAvailable
	usedPct := float64(used) / float64(m.MemTotal) * 100
	cat.Values = map[string]float64{"used_pct": usedPct}

	cat.Data = fmt.Sprintf("total=%dMB used=%.1f%% swap_total=%dMB swap_free=%dMB",
		m.MemTotal/1024, usedPct, m.SwapTotal/1024, m.SwapFree/1024)

	// MEMORY_PRESSURE / MEMORY_EXHAUSTION.
	switch {
	case usedPct >= 94:
		cat.AddSymptom(Symptom{ID: "MEMORY_EXHAUSTION", Level: LevelError,
			Summary: fmt.Sprintf("%.0f%% used", usedPct)})
	case usedPct >= 80:
		cat.AddSymptom(Symptom{ID: "MEMORY_PRESSURE", Level: LevelWarn,
			Summary: fmt.Sprintf("%.0f%% used", usedPct)})
	default:
		cat.AddSymptom(Symptom{ID: "MEMORY_PRESSURE", Level: LevelOK,
			Summary: fmt.Sprintf("%.0f%% used", usedPct)})
	}

	// SWAP_USAGE.
	if m.SwapTotal > 0 {
		swapUsed := m.SwapTotal - m.SwapFree
		swapPct := float64(swapUsed) / float64(m.SwapTotal) * 100
		switch {
		case swapPct >= 50:
			cat.AddSymptom(Symptom{ID: "SWAP_USAGE", Level: LevelError,
				Summary: fmt.Sprintf("%s used (%.0f%%)", bytesHuman(swapUsed*1024), swapPct)})
		case swapPct >= 20:
			cat.AddSymptom(Symptom{ID: "SWAP_USAGE", Level: LevelWarn,
				Summary: fmt.Sprintf("%s used", bytesHuman(swapUsed*1024))})
		case swapUsed > 0:
			cat.AddSymptom(Symptom{ID: "SWAP_USAGE", Level: LevelOK,
				Summary: fmt.Sprintf("%s used", bytesHuman(swapUsed*1024))})
		}

		// HIGH_SWAP_ACTIVITY: SwapCached относительно общего swap.
		if m.SwapCached > 0 && swapUsed > 0 {
			ratio := float64(m.SwapCached) / float64(swapUsed)
			if ratio > 0.5 {
				cat.AddSymptom(Symptom{ID: "HIGH_SWAP_ACTIVITY", Level: LevelWarn,
					Summary: fmt.Sprintf("%s cached in swap", bytesHuman(m.SwapCached*1024))})
			}
		}
	}

	// OOM_DETECTED: по журналу ядра.
	if oom, ok := detectOOM(); ok {
		cat.AddSymptom(Symptom{ID: "OOM_DETECTED", Level: LevelError, Summary: oom})
	}

	// PROCESS_MEMORY_HIGH: топ процессов по RSS.
	procs := processInfos()
	sort.Slice(procs, func(i, j int) bool { return procs[i].RSS > procs[j].RSS })
	added := 0
	for _, p := range procs {
		if p.RSS == 0 || p.Name == "" {
			continue
		}
		pctOfTotal := float64(p.RSS) / float64(m.MemTotal*1024) * 100
		if pctOfTotal < 10 {
			continue
		}
		lvl := LevelWarn
		if pctOfTotal >= 25 {
			lvl = LevelError
		}
		cat.AddSymptom(Symptom{
			ID:          "PROCESS_MEMORY_HIGH",
			Level:       lvl,
			ProcessName: p.Name,
			Summary:     fmt.Sprintf("%s: %s", p.Name, bytesHuman(p.RSS)),
			Detail:      fmt.Sprintf("%s (%d%%) of total RAM, PID %d, oom_score %d", bytesHuman(p.RSS), int(pctOfTotal), p.PID, p.OOMScore),
		})
		added++
		if added >= 5 {
			break
		}
	}

	// MEMORY_GROWTH: сравнение с предыдущим снапшотом.
	if prev != nil {
		if pc := prev.Category(CatMemory); pc != nil {
			if prevPct, ok := pc.Values["used_pct"]; ok && prevPct > 0 && usedPct-prevPct > 15 {
				cat.AddSymptom(Symptom{ID: "MEMORY_GROWTH", Level: LevelWarn,
					Summary: fmt.Sprintf("+%.0f pp since last snapshot (%.0f%% -> %.0f%%)", usedPct-prevPct, prevPct, usedPct)})
			}
		}
	}

	cat.Detected = true
	return cat
}

// detectOOM проверяет журнал ядра на сообщения OOM killer.
func detectOOM() (string, bool) {
	lines := tailJournal("kernel", 40)
	for _, l := range lines {
		low := strings.ToLower(l)
		if strings.Contains(low, "out of memory") || strings.Contains(low, "oom-kill") {
			return "OOM killer events in kernel log", true
		}
	}
	return "", false
}

// bytesHuman форматирует байты в читаемый вид.
func bytesHuman(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
