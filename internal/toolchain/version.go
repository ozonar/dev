package toolchain

import (
	"strconv"
	"strings"
)

// compareMajorMinor сравнивает две версии, учитывая только major и minor
// компоненты (например "1.22.2" и "1.22" считаются равными, так как major и
// minor совпадают). Возвращает 1, если a > b; -1 если a < b; 0 если равны.
// Используется реализациями Satisfies конкретных рантаймов (Go, Php).
func compareMajorMinor(a, b string) int {
	ai := parseMajorMinor(a)
	bi := parseMajorMinor(b)
	for i := 0; i < 2; i++ {
		if ai[i] > bi[i] {
			return 1
		}
		if ai[i] < bi[i] {
			return -1
		}
	}
	return 0
}

// parseMajorMinor разбирает версию на два целых числа — major и minor.
// Версия может содержать произвольное число компонентов ("1.22.2") и
// посторонние символы ("go1.22") — учитываются только первые две числовые
// части. Версии без второй части (например "8") получают minor = 0.
func parseMajorMinor(v string) [2]int {
	parts := strings.Split(v, ".")
	var res [2]int
	idx := 0
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		res[idx] = n
		idx++
		if idx == 2 {
			break
		}
	}
	return res
}

// compareFull сравнивает полные версии (major.minor.patch) по числовым
// компонентам. Возвращает 1, если a > b; -1 если a < b; 0 если равны.
// Используется для выбора самой свежей патч-версии среди списка кандидатов.
func compareFull(a, b string) int {
	ai := parseFull(a)
	bi := parseFull(b)
	for i := 0; i < 3; i++ {
		if ai[i] > bi[i] {
			return 1
		}
		if ai[i] < bi[i] {
			return -1
		}
	}
	return 0
}

// parseFull разбирает полную версию на три числа (major, minor, patch).
// Отсутствующие компоненты считаются нулевыми.
func parseFull(v string) [3]int {
	parts := strings.Split(v, ".")
	var res [3]int
	idx := 0
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		if idx >= 3 {
			break
		}
		res[idx] = n
		idx++
	}
	return res
}

// sortFullVersionsDesc сортирует полные версии (major.minor.patch)
// от новейшей к старейшей (in-place).
func sortFullVersionsDesc(versions []string) []string {
	for i := 1; i < len(versions); i++ {
		for j := i; j > 0 && compareFull(versions[j], versions[j-1]) > 0; j-- {
			versions[j], versions[j-1] = versions[j-1], versions[j]
		}
	}
	return versions
}
