package prod

import (
	"fmt"
	"strings"
	"syscall"
	"time"
)

// mountPoint представляет проверяемую точку монтирования.
type mountPoint struct {
	Path string
	Desc string
}

// defaultMounts возвращает список точек монтирования для проверки.
func defaultMounts() []mountPoint {
	return []mountPoint{
		{Path: "/", Desc: "root"},
	}
}

// collectDisk собирает категорию Disk.
func collectDisk(prev *Report) *Category {
	cat := &Category{ID: CatDisk, Present: true}

	worstUsed := 0.0
	var worstMnt string
	mounted := false
	for _, mnt := range defaultMounts() {
		var st syscall.Statfs_t
		if err := syscall.Statfs(mnt.Path, &st); err != nil {
			continue
		}
		mounted = true
		total := int64(st.Blocks) * int64(st.Bsize)
		free := int64(st.Bavail) * int64(st.Bsize)
		used := total - free
		usedPct := 0.0
		if total > 0 {
			usedPct = float64(used) / float64(total) * 100
		}
		if usedPct > worstUsed {
			worstUsed = usedPct
			worstMnt = mnt.Desc
		}

		// DISK_SPACE_EXHAUSTION.
		switch {
		case usedPct >= 92:
			cat.AddSymptom(Symptom{ID: "DISK_SPACE_EXHAUSTION", Level: LevelError,
				Summary: fmt.Sprintf("%s (%.0f%% used)", mnt.Desc, usedPct),
				Detail:  fmt.Sprintf("%s mount at %.0f%% used (%s free)", mnt.Desc, usedPct, bytesHuman(free))})
		case usedPct >= 80:
			cat.AddSymptom(Symptom{ID: "DISK_SPACE_EXHAUSTION", Level: LevelWarn,
				Summary: fmt.Sprintf("%s (%.0f%% used)", mnt.Desc, usedPct)})
		default:
			cat.AddSymptom(Symptom{ID: "DISK_SPACE_EXHAUSTION", Level: LevelOK,
				Summary: fmt.Sprintf("%s (%.0f%% used)", mnt.Desc, usedPct)})
		}

		// INODE_EXHAUSTION.
		if st.Files > 0 {
			inodePct := float64(st.Files-st.Ffree) / float64(st.Files) * 100
			switch {
			case inodePct >= 90:
				cat.AddSymptom(Symptom{ID: "INODE_EXHAUSTION", Level: LevelError,
					Summary: fmt.Sprintf("%s inodes (%.0f%% used)", mnt.Desc, inodePct)})
			case inodePct >= 80:
				cat.AddSymptom(Symptom{ID: "INODE_EXHAUSTION", Level: LevelWarn,
					Summary: fmt.Sprintf("%s inodes (%.0f%% used)", mnt.Desc, inodePct)})
			}
		}
	}

	if !mounted {
		// Ни одна точка монтирования не прочиталась — категорию не показываем.
		cat.Present = false
		return cat
	}
	cat.Data = fmt.Sprintf("worst_mount=%s used=%.1f%%", worstMnt, worstUsed)

	// DISK_SATURATION по /proc/diskstats (io_ticks delta / elapsed).
	if util, dev, ok := diskUtilization(); ok {
		switch {
		case util >= 90:
			cat.AddSymptom(Symptom{ID: "DISK_SATURATION", Level: LevelError,
				Summary: fmt.Sprintf("%s — %.0f%% busy", dev, util)})
		case util >= 70:
			cat.AddSymptom(Symptom{ID: "DISK_SATURATION", Level: LevelWarn,
				Summary: fmt.Sprintf("%s — %.0f%% busy", dev, util)})
		}
	}

	cat.Detected = true
	return cat
}

// diskUtilization вычисляет процент занятости самого нагруженного диска,
// измеряя приращение io_ticks за короткий интервал.
func diskUtilization() (float64, string, bool) {
	t0 := readDiskStats()
	time.Sleep(300 * time.Millisecond)
	t1 := readDiskStats()

	// Время интервала — фиксированные 300 мс (значение поля io_ticks в мс).
	intervalMS := 300.0
	bestUtil := 0.0
	bestDev := ""
	found := false
	for dev, a := range t0 {
		b, ok := t1[dev]
		if !ok {
			continue
		}
		deltaTicks := b.ioTicks - a.ioTicks
		if deltaTicks < 0 {
			continue
		}
		util := float64(deltaTicks) / intervalMS * 100
		if util > bestUtil {
			bestUtil = util
			bestDev = dev
			found = true
		}
	}
	return bestUtil, bestDev, found
}

type diskStatEntry struct {
	ioTicks int64
}

// readDiskStats читает /proc/diskstats и возвращает io_ticks по устройствам.
func readDiskStats() map[string]diskStatEntry {
	out := make(map[string]diskStatEntry)
	s, err := readFile("/proc/diskstats")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 14 {
			continue
		}
		// major minor device ... field 13 = time spent doing I/Os (ms)
		dev := f[2]
		ioTicks := parseInt(f[12])
		out[dev] = diskStatEntry{ioTicks: ioTicks}
	}
	return out
}
