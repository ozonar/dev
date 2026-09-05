// Package prod реализует диагностику продакшен-сервера: сбор данных о
// категориях (CPU, память, диск, БД, ...), определение симптомов и построение
// отчётов, в том числе причинных цепочек (cascading failure).
package prod

import (
	"strings"
	"time"
)

// Level — уровень серьёзности категории/симптома.
type Level string

const (
	LevelOK    Level = "ok"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
	LevelInfo  Level = "info"
)

// Label возвращает человекочитаемый лейбл уровня.
func (l Level) Label() string {
	switch l {
	case LevelOK:
		return "OK"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelInfo:
		return "?"
	default:
		return "OK"
	}
}

// CategoryID — уникальный идентификатор категории.
type CategoryID string

// Встроенный перечень категорий. Redis добавляется только при обнаружении.
const (
	CatCPU      CategoryID = "cpu"
	CatMemory   CategoryID = "memory"
	CatDisk     CategoryID = "disk"
	CatFD       CategoryID = "fd"
	CatNetwork  CategoryID = "network"
	CatPHPFPM   CategoryID = "php-fpm"
	CatPostgres CategoryID = "pgsql"
	CatRedis    CategoryID = "redis"
	CatExternal CategoryID = "external"
	CatDNS      CategoryID = "dns"
	CatHTTP     CategoryID = "http"
	CatRecent   CategoryID = "recent"
)

// Title возвращает отображаемое название категории.
func (id CategoryID) Title() string {
	switch id {
	case CatCPU:
		return "CPU"
	case CatMemory:
		return "Memory"
	case CatDisk:
		return "Disk"
	case CatFD:
		return "File descriptors"
	case CatNetwork:
		return "Network"
	case CatPHPFPM:
		return "PHP-FPM"
	case CatPostgres:
		return "Database"
	case CatRedis:
		return "Redis"
	case CatExternal:
		return "External dependencies"
	case CatDNS:
		return "DNS"
	case CatHTTP:
		return "HTTP"
	case CatRecent:
		return "Recent changes"
	default:
		return string(id)
	}
}

// Symptom представляет один выявленный симптом.
type Symptom struct {
	ID          string  `json:"id"`                     // напр. CPU_SATURATION
	Level       Level   `json:"level"`                  // серьёзность симптома
	Summary     string  `json:"summary"`                // краткое описание, напр. "97% — high"
	Detail      string  `json:"detail,omitempty"`       // подробное описание (для detail-отчёта)
	ProcessName string  `json:"process_name,omitempty"` // связанный процесс (если есть)
	Confidence  float64 `json:"confidence,omitempty"`   // уверенность 0..1 (для кандидатов)
	Value       float64 `json:"value,omitempty"`        // числовая метрика для сравнения снапшотов
}

// Category представляет одну категорию проверки.
type Category struct {
	ID       CategoryID         `json:"id"`
	Level    Level              `json:"level"`
	Present  bool               `json:"present"`  // false — не обнаружено (Redis нет) -> не показываем
	Detected bool               `json:"detected"` // удалось ли собрать данные
	Data     string             `json:"data,omitempty"`
	Values   map[string]float64 `json:"values,omitempty"` // числовые метрики категории для сравнения снапшотов
	Symptoms []Symptom          `json:"symptoms"`
}

// AddSymptom добавляет симптом и повышает уровень категории при необходимости.
func (c *Category) AddSymptom(s Symptom) {
	c.Symptoms = append(c.Symptoms, s)
	if levelWeight(s.Level) > levelWeight(c.Level) {
		c.Level = s.Level
	}
}

func levelWeight(l Level) int {
	switch l {
	case LevelError:
		return 3
	case LevelWarn:
		return 2
	case LevelOK:
		return 1
	default:
		return 0
	}
}

// Report — полный отчёт о состоянии продакшена в момент снятия снапшота.
type Report struct {
	Timestamp  time.Time  `json:"timestamp"`
	Hostname   string     `json:"hostname"`
	Categories []Category `json:"categories"`
}

// RootCandidate — кандидат в корневую причину.
type RootCandidate struct {
	Name       string  `json:"name"`
	Confidence string  `json:"confidence"` // HIGH / MEDIUM / LOW
	Score      float64 `json:"score"`      // 0..1
}

// ChainLink — звено причинной цепочки.
type ChainLink struct {
	Label    string     `json:"label"`
	Detail   string     `json:"detail,omitempty"`
	Category CategoryID `json:"category"`
}

// Cascade — результат построения причинной цепочки.
type Cascade struct {
	Chain []ChainLink     `json:"chain"`
	Roots []RootCandidate `json:"root_candidates"`
}

// Summary возвращает категорию по id, либо nil.
func (r *Report) Category(id CategoryID) *Category {
	for i := range r.Categories {
		if r.Categories[i].ID == id {
			return &r.Categories[i]
		}
	}
	return nil
}

// OverallLevel возвращает наивысший уровень по всем категориям.
func (r *Report) OverallLevel() Level {
	lvl := LevelOK
	for _, c := range r.Categories {
		if c.Present && levelWeight(c.Level) > levelWeight(lvl) {
			lvl = c.Level
		}
	}
	return lvl
}

// CategoryByName сопоставляет строковое имя с идентификатором категории.
// Поддерживает основные имена и алиасы.
func CategoryByName(name string) (CategoryID, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	aliases := map[string]CategoryID{
		"cpu":        CatCPU,
		"memory":     CatMemory,
		"mem":        CatMemory,
		"disk":       CatDisk,
		"fd":         CatFD,
		"file":       CatFD,
		"network":    CatNetwork,
		"net":        CatNetwork,
		"php-fpm":    CatPHPFPM,
		"phpfpm":     CatPHPFPM,
		"fpm":        CatPHPFPM,
		"pgsql":      CatPostgres,
		"postgres":   CatPostgres,
		"postgresql": CatPostgres,
		"database":   CatPostgres,
		"db":         CatPostgres,
		"redis":      CatRedis,
		"external":   CatExternal,
		"deps":       CatExternal,
		"recent":     CatRecent,
	}
	id, ok := aliases[n]
	return id, ok
}

// AvailableCategories возвращает список доступных категорий.
func AvailableCategories() string {
	ids := []CategoryID{CatCPU, CatMemory, CatDisk, CatFD, CatNetwork,
		CatPHPFPM, CatPostgres, CatRedis, CatExternal, CatRecent}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, string(id))
	}
	return strings.Join(names, ", ")
}
