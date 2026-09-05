package prod

import (
	"fmt"
	"strings"

	"dev/internal/colors"
)

// levelColor возвращает цветную функцию для уровня.
func levelColor(l Level) func(string) string {
	switch l {
	case LevelError:
		return colors.Red
	case LevelWarn:
		return colors.Yellow
	case LevelInfo:
		return colors.Cyan
	default:
		return colors.Green
	}
}

// levelLabel красит лейбл уровня в соответствующий цвет.
func levelLabel(l Level) string {
	return levelColor(l)("[" + l.Label() + "]")
}

// symptomLabel красит ID симптома серым.
func symptomLabel(id string) string {
	return colors.Gray("[" + id + "]")
}

// RenderBrief выводит краткий отчёт: по одной значимой строке на категорию.
func RenderBrief(rep *Report) {
	var sb strings.Builder
	for i := range rep.Categories {
		c := &rep.Categories[i]
		sb.WriteString(levelLabel(c.Level))
		sb.WriteString(" ")
		sb.WriteString(c.ID.Title())
		sb.WriteString("\n")

		// Выбираем наиболее значимый симптом (самый серьёзный).
		top := mostSevere(c.Symptoms)
		if top == nil {
			continue
		}
		// Для не-OK категорий показываем ID симптома.
		if c.Level == LevelError || c.Level == LevelWarn {
			sb.WriteString("\t")
			sb.WriteString(symptomLabel(top.ID))
			sb.WriteString("\n")
			sb.WriteString("\t")
			sb.WriteString(colors.White(top.Summary))
		} else {
			sb.WriteString("\t")
			sb.WriteString(colors.White(top.Summary))
		}
		sb.WriteString("\n\n")
	}
	fmt.Print(sb.String())
}

// RenderDetail выводит подробный отчёт: все симптомы и детали.
func RenderDetail(rep *Report) {
	var sb strings.Builder
	for i := range rep.Categories {
		c := &rep.Categories[i]
		sb.WriteString(levelLabel(c.Level))
		sb.WriteString(" ")
		sb.WriteString(c.ID.Title())
		sb.WriteString("\n")

		if len(c.Symptoms) == 0 {
			sb.WriteString("\tno data\n\n")
			continue
		}
		for _, s := range c.Symptoms {
			lvl := levelColor(s.Level)
			sb.WriteString("    ")
			sb.WriteString(symptomLabel(s.ID))
			sb.WriteString("\n")
			line := "        " + lvl(s.Summary)
			if s.Detail != "" {
				line += "  " + colors.Gray("("+s.Detail+")")
			}
			sb.WriteString(line)
			sb.WriteString("\n\n")
		}
	}
	fmt.Print(sb.String())
}

// RenderCategory выводит подробный отчёт по одной категории.
func RenderCategory(cat *Category) {
	rep := &Report{Categories: []Category{*cat}}
	RenderDetail(rep)
}

// mostSevere возвращает симптом с наивысшим уровнем серьёзности.
func mostSevere(syms []Symptom) *Symptom {
	if len(syms) == 0 {
		return nil
	}
	best := &syms[0]
	for i := range syms {
		if levelWeight(syms[i].Level) > levelWeight(best.Level) {
			best = &syms[i]
		}
	}
	return best
}
