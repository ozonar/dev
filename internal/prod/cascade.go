package prod

import (
	"dev/internal/colors"
	"fmt"
	"sort"
)

// chainNode описывает один узел причинной цепочки с приоритетом причинности.
type chainNode struct {
	Category  CategoryID
	Label     string
	RootScore float64 // насколько вероятно, что это корневая причина (0..1)
}

// categoryRootScore — базовая причинность категории: что обычно первопричина.
var categoryRootScore = map[CategoryID]float64{
	CatExternal: 0.95,
	CatRecent:   0.9,
	CatDNS:      0.85,
	CatRedis:    0.75,
	CatPostgres: 0.7,
	CatHTTP:     0.6,
	CatNetwork:  0.55,
	CatCPU:      0.5,
	CatMemory:   0.5,
	CatFD:       0.5,
	CatDisk:     0.45,
	CatPHPFPM:   0.35,
}

// buildChainNodes формирует упорядоченный список узлов по обнаруженным
// симптомам. Порядок задаёт причинность: от корня к следствиям.
func buildChainNodes(rep *Report) []chainNode {
	var nodes []chainNode
	for i := range rep.Categories {
		c := &rep.Categories[i]
		top := mostSevere(c.Symptoms)
		if top == nil || top.Level == LevelOK || top.Level == LevelInfo {
			continue
		}
		label := top.Summary
		nodes = append(nodes, chainNode{
			Category:  c.ID,
			Label:     label,
			RootScore: rootScore(c.ID, top),
		})
	}

	// Сортируем по убыванию "корневости", чтобы самые вероятные причины были
	// в начале цепочки.
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].RootScore > nodes[j].RootScore
	})
	return nodes
}

// rootScore оценивает вероятность того, что категория — корневая причина.
func rootScore(id CategoryID, s *Symptom) float64 {
	base := categoryRootScore[id]

	// Чем серьёзнее симптом, тем выше шанс, что это реальная причина.
	sev := 0.0
	switch s.Level {
	case LevelError:
		sev = 0.15
	case LevelWarn:
		sev = 0.05
	}
	return base + sev
}

// confidenceLabel переводит score в текстовую уверенность.
func confidenceLabel(score float64) string {
	switch {
	case score >= 0.7:
		return "HIGH"
	case score >= 0.5:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

// BuildCascade строит причинную цепочку и кандидатов в корневые причины.
func BuildCascade(rep *Report) Cascade {
	nodes := buildChainNodes(rep)

	chain := make([]ChainLink, 0, len(nodes))
	for _, n := range nodes {
		chain = append(chain, ChainLink{
			Label:    n.Label,
			Category: n.Category,
		})
	}

	var roots []RootCandidate
	// Кандидаты — первые узлы цепочки (наиболее вероятные причины).
	for i, n := range nodes {
		if i >= 4 {
			break
		}
		roots = append(roots, RootCandidate{
			Name:       fmt.Sprintf("%s %s", n.Category.Title(), shortLabel(n.Label)),
			Confidence: confidenceLabel(n.RootScore),
			Score:      n.RootScore,
		})
	}

	return Cascade{Chain: chain, Roots: roots}
}

func shortLabel(label string) string {
	// Берём первую часть label без слишком длинных деталей.
	if len(label) > 40 {
		return label[:37] + "..."
	}
	return label
}

// RenderCascade выводит цепочку стрелками и список корневых кандидатов.
func RenderCascade(c Cascade) {
	if len(c.Chain) == 0 {
		fmt.Println(levelColor(LevelInfo)("No active anomalies detected — no cascade."))
		return
	}
	// Первый узел сверху, далее вниз стрелкой ↑ в обратном направлении не нужно.
	// По спецификации: первопричина сверху, следствия вниз.
	levels := make([]string, len(c.Chain))
	for i, l := range c.Chain {
		levels[i] = l.Label
	}

	fmt.Println()
	for i, label := range levels {
		fmt.Println("  " + colors.White(label))
		if i < len(levels)-1 {
			fmt.Println("        ↑")
			fmt.Println("        │")
		}
	}

	if len(c.Roots) > 0 {
		fmt.Println("\n  ROOT CANDIDATES")
		for i, r := range c.Roots {
			conf := levelColor(confidenceLevel(r.Confidence))("(" + r.Confidence + " confidence)")
			fmt.Printf("  %d. %-40s %s\n", i+1, r.Name, conf)
		}
	}
	fmt.Println()
}

func confidenceLevel(conf string) Level {
	switch conf {
	case "HIGH":
		return LevelError
	case "MEDIUM":
		return LevelWarn
	default:
		return LevelInfo
	}
}
