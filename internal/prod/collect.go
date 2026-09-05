package prod

import (
	"os"
	"sort"
	"time"
)

// Collect выполняет сбор полного отчёта о состоянии продакшена.
// prev — предыдущий снапшот (может быть nil), используется для трендов.
func Collect(prev *Report) *Report {
	hostname, _ := os.Hostname()
	rep := &Report{
		Timestamp: time.Now(),
		Hostname:  hostname,
	}

	cats := []*Category{
		collectCPU(prev),
		collectMemory(prev),
		collectDisk(prev),
		collectFD(prev),
		collectNetwork(prev),
		collectPHPFPM(prev),
		collectPostgres(prev),
		collectRedis(prev),
		collectExternal(prev),
		collectRecent(prev),
	}

	for _, c := range cats {
		// Скрываем категории: не обнаруженные, либо не давшие ни одного симптома
		// (пустые — напр. Recent changes без деплоя, External без зависимостей).
		if !c.Present || len(c.Symptoms) == 0 {
			continue
		}
		rep.Categories = append(rep.Categories, *c)
	}

	// Сортируем по уровню серьёзности (ошибки первыми).
	sort.SliceStable(rep.Categories, func(i, j int) bool {
		return levelWeight(rep.Categories[i].Level) > levelWeight(rep.Categories[j].Level)
	})

	return rep
}
