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

// categoryDef — единое описание категории: идентификатор, отображаемое имя,
// алиасы для CLI и функция сбора. collect == nil означает, что категория
// участвует только в причинных весах и не собирается отдельно.
type categoryDef struct {
	id      CategoryID
	title   string
	aliases []string
	collect func(*Report) *Category
}

// categoryDefs — единственный реестр категорий prod. Из него строятся
// отображаемые имена, алиасы, список доступных категорий и порядок сбора.
var categoryDefs = []categoryDef{
	{id: CatCPU, title: "CPU", aliases: []string{"cpu"}, collect: collectCPU},
	{id: CatMemory, title: "Memory", aliases: []string{"memory", "mem"}, collect: collectMemory},
	{id: CatDisk, title: "Disk", aliases: []string{"disk"}, collect: collectDisk},
	{id: CatFD, title: "File descriptors", aliases: []string{"fd", "file"}, collect: collectFD},
	{id: CatNetwork, title: "Network", aliases: []string{"network", "net"}, collect: collectNetwork},
	{id: CatPHPFPM, title: "PHP-FPM", aliases: []string{"php-fpm", "phpfpm", "fpm"}, collect: collectPHPFPM},
	{id: CatPostgres, title: "Database", aliases: []string{"pgsql", "postgres", "postgresql", "database", "db"}, collect: collectPostgres},
	{id: CatRedis, title: "Redis", aliases: []string{"redis"}, collect: collectRedis},
	{id: CatExternal, title: "External dependencies", aliases: []string{"external", "deps"}, collect: collectExternal},
	{id: CatDNS, title: "DNS", aliases: []string{"dns"}},
	{id: CatHTTP, title: "HTTP", aliases: []string{"http"}},
	{id: CatRecent, title: "Recent changes", aliases: []string{"recent"}, collect: collectRecent},
}

// Title возвращает отображаемое название категории.
func (id CategoryID) Title() string {
	for _, def := range categoryDefs {
		if def.id == id {
			return def.title
		}
	}
	return string(id)
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
// Поддерживает основные имена и алиасы из реестра категорий.
func CategoryByName(name string) (CategoryID, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, def := range categoryDefs {
		if n == strings.ToLower(string(def.id)) {
			return def.id, true
		}
		for _, alias := range def.aliases {
			if n == alias {
				return def.id, true
			}
		}
	}
	return "", false
}

// AvailableCategories возвращает список категорий, которые можно собрать.
func AvailableCategories() string {
	names := make([]string, 0, len(categoryDefs))
	for _, def := range categoryDefs {
		if def.collect != nil {
			names = append(names, string(def.id))
		}
	}
	return strings.Join(names, ", ")
}
