package prod

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// cpuSample — снимок /proc/stat и всех процессов в один момент времени.
type cpuSample struct {
	total  float64 // суммарные jiffies по строке cpu
	user   float64
	system float64
	iowait float64
	steal  float64
	procs  map[int]processInfo
}

// collectCPUSample снимает состояние CPU и процессов.
func collectCPUSample() cpuSample {
	s := cpuSample{procs: make(map[int]processInfo)}
	if txt, err := readFile("/proc/stat"); err == nil {
		for _, line := range strings.Split(txt, "\n") {
			if !strings.HasPrefix(line, "cpu") {
				continue
			}
			if len(line) > 3 && line[3] != ' ' {
				continue // cpu0, cpu1, ... — пропускаем агрегат только для cpu
			}
			f := strings.Fields(line)
			if len(f) < 5 {
				break
			}
			var vals []float64
			for _, v := range f[1:] {
				vals = append(vals, parseFloat(v))
			}
			s.total = sum(vals)
			if len(vals) > 0 {
				s.user = vals[0]
			}
			if len(vals) > 2 {
				s.system = vals[2]
			}
			if len(vals) > 4 {
				s.iowait = vals[4]
			}
			if len(vals) > 8 {
				s.steal = vals[8]
			}
			break
		}
	}
	for _, pi := range processInfos() {
		s.procs[pi.PID] = pi
	}
	return s
}

func sum(vals []float64) float64 {
	var t float64
	for _, v := range vals {
		t += v
	}
	return t
}

// cpuPercent переводит разницу jiffies в проценты одного ядра.
func cpuPercent(deltaJiffies, deltaTotal float64, cores int) float64 {
	if deltaTotal <= 0 || cores <= 0 {
		return 0
	}
	return (deltaJiffies / deltaTotal) * float64(cores) * 100
}

// collectCPU собирает категорию CPU. Для расчёта используется два замера с
// небольшой паузой.
func collectCPU(prev *Report) *Category {
	cat := &Category{ID: CatCPU, Present: true}

	t0 := collectCPUSample()
	time.Sleep(300 * time.Millisecond)
	t1 := collectCPUSample()

	cores := cpuCount()
	if cores == 0 {
		cores = 1
	}

	deltaTotal := t1.total - t0.total
	userPct := cpuPercent(t1.user-t0.user, deltaTotal, cores)
	sysPct := cpuPercent(t1.system-t0.system, deltaTotal, cores)
	ioPct := cpuPercent(t1.iowait-t0.iowait, deltaTotal, cores)
	stealPct := cpuPercent(t1.steal-t0.steal, deltaTotal, cores)
	totalPct := userPct + sysPct

	// Используем максимальное из мгновенного total и loadavg как оценку насыщения.
	load := loadAvg()
	loadPerCore := load[0] / float64(cores)

	cat.Data = fmt.Sprintf("total=%.1f%% user=%.1f%% sys=%.1f%% io=%.1f%% steal=%.1f%% load1=%.2f cores=%d",
		totalPct, userPct, sysPct, ioPct, stealPct, load[0], cores)

	// CPU_SATURATION: либо высокая утилизация, либо loadavg на уровне ядер.
	satPct := totalPct
	if loadPerCore > 0.7 && satPct < loadPerCore*100 {
		satPct = loadPerCore * 100
	}
	totalCap := satPct / float64(cores)
	switch {
	case satPct >= 90:
		cat.AddSymptom(Symptom{ID: "CPU_SATURATION", Level: LevelError,
			Summary: fmt.Sprintf("%.0f%% of %d cores — high", totalCap, cores)})
	case satPct >= 70:
		cat.AddSymptom(Symptom{ID: "CPU_SATURATION", Level: LevelWarn,
			Summary: fmt.Sprintf("%.0f%% of %d cores — elevated", totalCap, cores)})
	default:
		cat.AddSymptom(Symptom{ID: "CPU_SATURATION", Level: LevelOK,
			Summary: fmt.Sprintf("%.0f%% of %d cores — normal", totalCap, cores)})
	}

	// Высокие user/system/iowait/steal компоненты.
	addComponent := func(id string, pct float64, label string) {
		if pct >= 85 {
			cat.AddSymptom(Symptom{ID: id, Level: LevelWarn, Summary: fmt.Sprintf("%s — %.0f%%", label, pct)})
		}
	}
	addComponent("HIGH_USER_CPU", userPct, "user")
	addComponent("HIGH_SYSTEM_CPU", sysPct, "system")
	addComponent("HIGH_IOWAIT", ioPct, "iowait")
	addComponent("HIGH_CPU_STEAL", stealPct, "steal")

	// PROCESS_CPU_HIGH: топ потребителей CPU за интервал.
	type procUsage struct {
		name string
		pid  int
		pct  float64
	}
	var usages []procUsage
	for pid, p0 := range t0.procs {
		p1, ok := t1.procs[pid]
		if !ok {
			continue
		}
		d := p1.CPU - p0.CPU
		if d < 0 {
			continue // процесс перезапустился — счётчик сброшен
		}
		pct := cpuPercent(d, deltaTotal, cores)
		if pct >= 5 {
			name := p1.Name
			if name == "" {
				name = fmt.Sprintf("pid %d", pid)
			}
			usages = append(usages, procUsage{name: name, pid: pid, pct: pct})
		}
	}
	sort.Slice(usages, func(i, j int) bool { return usages[i].pct > usages[j].pct })
	for i, u := range usages {
		if i >= 5 {
			break
		}
		lvl := LevelWarn
		if u.pct >= 50 {
			lvl = LevelError
		}
		cat.AddSymptom(Symptom{
			ID:          "PROCESS_CPU_HIGH",
			Level:       lvl,
			ProcessName: u.name,
			Summary:     fmt.Sprintf("%s (PID %d) — %.0f%% (%.0f%% of %d cores)", u.name, u.pid, u.pct, u.pct/float64(cores), cores),
		})
	}

	cat.Detected = true
	return cat
}
