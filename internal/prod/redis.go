package prod

import (
	"fmt"
	"strings"
)

// redisDetected проверяет наличие Redis на сервере.
func redisDetected() bool {
	if isPortOpen("127.0.0.1:6379") {
		return true
	}
	// Ищем процесс redis-server.
	for _, p := range processInfos() {
		if strings.Contains(strings.ToLower(p.Name), "redis") {
			return true
		}
	}
	return false
}

// redisInfo собирает пары key:value из redis-cli INFO.
func redisInfo() map[string]string {
	out := execOut("sh", "-c", "redis-cli --no-auth-warning INFO 2>/dev/null || redis-cli INFO 2>/dev/null")
	if out == "" {
		return nil
	}
	res := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "#") {
			parts := strings.SplitN(line, ":", 2)
			res[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return res
}

// collectRedis собирает категорию Redis (только если Redis обнаружен).
func collectRedis(prev *Report) *Category {
	cat := &Category{ID: CatRedis, Present: true}
	if !redisDetected() {
		cat.Present = false
		return cat
	}
	info := redisInfo()
	if info == nil {
		cat.Detected = false
		return cat
	}
	cat.Detected = true
	cat.Data = "redis detected"

	// REDIS_MEMORY_PRESSURE.
	if used, ok := parseRedisBytes(info["used_memory"]); ok {
		if maxmem, ok2 := parseRedisBytes(info["maxmemory"]); ok2 && maxmem > 0 {
			pct := float64(used) / float64(maxmem) * 100
			switch {
			case pct >= 90:
				cat.AddSymptom(Symptom{ID: "REDIS_MEMORY_PRESSURE", Level: LevelError,
					Summary: fmt.Sprintf("%s / %s (%.0f%%)", bytesHuman(used), bytesHuman(maxmem), pct)})
			case pct >= 75:
				cat.AddSymptom(Symptom{ID: "REDIS_MEMORY_PRESSURE", Level: LevelWarn,
					Summary: fmt.Sprintf("%s / %s (%.0f%%)", bytesHuman(used), bytesHuman(maxmem), pct)})
			default:
				cat.AddSymptom(Symptom{ID: "REDIS_MEMORY_PRESSURE", Level: LevelOK,
					Summary: fmt.Sprintf("%s used", bytesHuman(used))})
			}
		}
	}

	// REDIS_CONNECTION_EXHAUSTION.
	if conns := parseInt(info["connected_clients"]); conns > 500 {
		lvl := LevelWarn
		if conns > 1000 {
			lvl = LevelError
		}
		cat.AddSymptom(Symptom{ID: "REDIS_CONNECTION_EXHAUSTION", Level: lvl,
			Summary: fmt.Sprintf("%d connected clients", conns)})
	}

	// REDIS_CACHE_HIT_RATE_DROP: сравнение hit rate с прошлым снапшотом.
	hits := parseInt(info["keyspace_hits"])
	misses := parseInt(info["keyspace_misses"])
	total := hits + misses
	if total > 0 {
		rate := float64(hits) / float64(total) * 100
		if prev != nil {
			if pc := prev.Category(CatRedis); pc != nil {
				for _, s := range pc.Symptoms {
					if s.ID == "REDIS_CACHE_HIT_RATE" && s.Value > 0 && rate < s.Value-10 {
						cat.AddSymptom(Symptom{ID: "REDIS_CACHE_HIT_RATE_DROP", Level: LevelWarn,
							Summary: fmt.Sprintf("hit rate dropped %.0f%% -> %.0f%%", s.Value, rate)})
					}
				}
			}
		}
		cat.AddSymptom(Symptom{ID: "REDIS_CACHE_HIT_RATE", Level: LevelOK,
			Summary: fmt.Sprintf("%.0f%% cache hit rate", rate), Value: rate})
	}

	return cat
}

// parseRedisBytes преобразует байтовое значение (может быть число или "1G").
func parseRedisBytes(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if n := parseInt(s); n > 0 {
		return n, true
	}
	// Поддержка суффиксов K/M/G.
	if len(s) < 2 {
		return 0, false
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'K', 'k':
		mult = 1024
	case 'M', 'm':
		mult = 1024 * 1024
	case 'G', 'g':
		mult = 1024 * 1024 * 1024
	default:
		return parseInt(s), true
	}
	num := parseFloat(s[:len(s)-1])
	return int64(num * float64(mult)), true
}
