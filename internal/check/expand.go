package check

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"dev/internal/ai"
)

// regionMergeGap — максимальное количество строк между изменёнными строками,
// при котором они считаются одним регионом. Близкие изменения раскрываются
// одной функцией, а не несколькими перекрывающимися блоками.
const regionMergeGap = 3

// declarationPrefixes — начала строк, которые считаются объявлением функции
// или типа. Используется для поиска границы блока без парсера, поэтому
// список покрывает основные языки проекта: PHP, Go, JavaScript, Python, Ruby.
var declarationPrefixes = []string{
	"func ",
	"function ",
	"async function ",
	"export function ",
	"export default function ",
	"def ",
	"async def ",
	"class ",
	"export class ",
	"interface ",
	"trait ",
	"struct ",
	"enum ",
	"impl ",
	"module ",
	"fn ",
	"sub ",
}

// diffFileChanges — изменения одного файла из unified diff: путь и номера
// изменённых строк в новой версии файла.
type diffFileChanges struct {
	path  string
	lines []int
}

// functionBlock — диапазон строк файла (0-based, включительно), который
// расширяет изменённый регион до границ функции.
type functionBlock struct {
	path  string
	start int
	end   int
}

// expandChangesToFunctions дополняет текст изменений целыми функциями вокруг
// изменённых строк в пределах бюджета. Базовый текст изменений сохраняется:
// сначала он сам укладывается в бюджет, а остаток распределяется между
// функциями. Возвращает итоговый текст и количество добавленных функций.
// Если текст не является unified diff, расширять нечего — возвращается он сам.
func expandChangesToFunctions(changes string, budget int) (string, int) {
	base := changes
	if len([]rune(base)) > budget {
		base = ai.ClampToChars(base, budget)
	}
	if !strings.Contains(changes, "@@ ") {
		return base, 0
	}

	remaining := budget - len([]rune(base))
	if remaining <= 0 {
		return base, 0
	}

	var sb strings.Builder
	sb.WriteString(base)
	added := 0

	for _, fc := range parseChangedLines(changes) {
		lines, ok := readFileLines(fc.path)
		if !ok {
			continue
		}
		for _, block := range fileFunctionBlocks(fc.path, lines, fc.lines) {
			text := functionBlockText(block, lines)
			// +2 — разделитель между блоками.
			cost := len([]rune(text)) + 2
			if cost > remaining {
				continue
			}
			sb.WriteString("\n\n")
			sb.WriteString(text)
			remaining -= cost
			added++
		}
	}

	return sb.String(), added
}

// parseChangedLines разбирает unified diff и возвращает изменённые файлы
// в порядке появления в diff с номерами изменённых строк новой версии.
func parseChangedLines(changes string) []diffFileChanges {
	var result []diffFileChanges
	expectPath := false
	inHunk := false
	newLine := 0
	currentIdx := -1

	for _, line := range strings.Split(changes, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			expectPath = false
			inHunk = false
			currentIdx = -1
		case !inHunk && strings.HasPrefix(line, "--- "):
			expectPath = true
		case !inHunk && expectPath && strings.HasPrefix(line, "+++ "):
			expectPath = false
			currentIdx = -1
			path := parseDiffPath(strings.TrimPrefix(line, "+++ "))
			if path != "" {
				result = append(result, diffFileChanges{path: path})
				currentIdx = len(result) - 1
			}
		case !inHunk && strings.HasPrefix(line, "@@ "):
			expectPath = false
			inHunk = currentIdx >= 0
			if inHunk {
				newLine = parseHunkNewStart(line)
			}
		case !inHunk:
			continue
		case strings.HasPrefix(line, "+"):
			result[currentIdx].lines = append(result[currentIdx].lines, newLine)
			newLine++
		case strings.HasPrefix(line, " "):
			newLine++
		}
	}

	return result
}

// parseDiffPath извлекает путь из строки заголовка +++ diff. Отбрасывает
// временную метку после таба, обрамляющие кавычки и префикс b/.
// Пустой результат означает /dev/null (файл удалён).
func parseDiffPath(s string) string {
	if i := strings.IndexByte(s, '\t'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"")
	if s == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(s, "b/")
}

// parseHunkNewStart извлекает начальный номер строки новой версии из
// заголовка ханка вида "@@ -1,2 +3,4 @@".
func parseHunkNewStart(line string) int {
	plus := strings.IndexByte(line, '+')
	if plus < 0 {
		return 0
	}
	rest := line[plus+1:]
	if end := strings.IndexAny(rest, ", "); end >= 0 {
		rest = rest[:end]
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0
	}
	return n
}

// readFileLines читает файл и разбивает его на строки. Бинарные и слишком
// большие файлы пропускаются: их содержимое не является исходным кодом.
func readFileLines(path string) ([]string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > ai.MaxReviewFileSize {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	if strings.ContainsRune(string(data), 0) {
		return nil, false
	}
	return strings.Split(string(data), "\n"), true
}

// fileFunctionBlocks вычисляет функции, покрывающие изменённые строки файла.
// Пересекающиеся и смежные функции объединяются, чтобы один код не попадал
// в контекст дважды.
func fileFunctionBlocks(path string, lines []string, changed []int) []functionBlock {
	regions := mergeLineRegions(changed)
	blocks := make([]functionBlock, 0, len(regions))
	for _, region := range regions {
		firstStart, firstEnd := functionBounds(lines, region[0]-1)
		lastStart, lastEnd := functionBounds(lines, region[1]-1)
		start, end := firstStart, firstEnd
		if lastStart < start {
			start = lastStart
		}
		if lastEnd > end {
			end = lastEnd
		}
		blocks = append(blocks, functionBlock{path: path, start: start, end: end})
	}
	return mergeFunctionBlocks(blocks)
}

// mergeLineRegions группирует номера строк в регионы: близкие строки
// (разрыв не больше regionMergeGap) попадают в один регион.
func mergeLineRegions(changed []int) [][2]int {
	if len(changed) == 0 {
		return nil
	}
	sorted := append([]int(nil), changed...)
	sort.Ints(sorted)

	var regions [][2]int
	start, end := sorted[0], sorted[0]
	for _, line := range sorted[1:] {
		if line <= end+regionMergeGap {
			end = line
			continue
		}
		regions = append(regions, [2]int{start, end})
		start, end = line, line
	}
	return append(regions, [2]int{start, end})
}

// mergeFunctionBlocks сортирует блоки и объединяет пересекающиеся и смежные.
func mergeFunctionBlocks(blocks []functionBlock) []functionBlock {
	if len(blocks) == 0 {
		return nil
	}
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].path != blocks[j].path {
			return blocks[i].path < blocks[j].path
		}
		return blocks[i].start < blocks[j].start
	})

	merged := []functionBlock{blocks[0]}
	for _, block := range blocks[1:] {
		last := &merged[len(merged)-1]
		if block.path == last.path && block.start <= last.end+1 {
			if block.end > last.end {
				last.end = block.end
			}
			continue
		}
		merged = append(merged, block)
	}
	return merged
}

// functionBounds возвращает границы функции, содержащей строку idx (0-based).
func functionBounds(lines []string, idx int) (int, int) {
	if len(lines) == 0 {
		return 0, 0
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(lines) {
		idx = len(lines) - 1
	}
	start := findBlockStart(lines, idx)
	end := findBlockEnd(lines, start)
	if end < start {
		end = start
	}
	return start, end
}

// findBlockStart ищет начало функции: ближайшее объявление выше изменённой
// строки. Если объявления нет, возвращает начало ближайшего внешнего блока.
func findBlockStart(lines []string, idx int) int {
	changeIndent := indentWidth(lines[idx])
	candidate := 0
	for i := idx; i >= 0; i-- {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		ind := indentWidth(line)
		if ind > changeIndent {
			continue
		}
		if isDeclaration(line) {
			return i
		}
		if ind < changeIndent {
			candidate = i + 1
		}
	}
	return candidate
}

// findBlockEnd ищет конец функции: сначала по балансу фигурных скобок,
// затем по отступам для языков без скобок.
func findBlockEnd(lines []string, start int) int {
	if end, ok := braceBlockEnd(lines, start); ok {
		return end
	}
	return indentBlockEnd(lines, start)
}

// braceBlockEnd возвращает строку, на которой баланс фигурных скобок,
// открытых в функции, вернулся к нулю.
func braceBlockEnd(lines []string, start int) (int, bool) {
	depth := 0
	opened := false
	for i := start; i < len(lines); i++ {
		for _, r := range lines[i] {
			switch r {
			case '{':
				depth++
				opened = true
			case '}':
				depth--
			}
		}
		if opened && depth <= 0 {
			return i, true
		}
	}
	return 0, false
}

// indentBlockEnd возвращает последнюю строку блока по отступам: блок
// заканчивается перед первой непустой строкой с отступом не больше базового.
func indentBlockEnd(lines []string, start int) int {
	base := indentWidth(lines[start])
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if indentWidth(line) <= base {
			return i - 1
		}
	}
	return len(lines) - 1
}

// isDeclaration сообщает, что строка объявляет функцию или тип.
func isDeclaration(line string) bool {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range declarationPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// indentWidth считает отступ строки в пробелах. Таб приравнивается к четырём
// пробелам: точная ширина не важна, важно сравнение отступов между строками.
func indentWidth(line string) int {
	width := 0
	for _, r := range line {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 4
		default:
			return width
		}
	}
	return width
}

// functionBlockText формирует текст блока с заголовком, по которому видно
// файл и диапазон строк. Каждая строка пронумерована по новой версии файла,
// чтобы модель ссылалась на готовый номер, а не вычисляла его.
func functionBlockText(block functionBlock, lines []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "===== %s (function context, lines %d-%d) =====\n",
		filepath.ToSlash(block.path), block.start+1, block.end+1)
	for i := block.start; i <= block.end && i < len(lines); i++ {
		fmt.Fprintf(&sb, "%d| %s\n", i+1, lines[i])
	}
	return sb.String()
}
