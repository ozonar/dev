package toolchain

import (
	"strconv"
	"strings"
)

// compareMajorMinor сравнивает две версии, учитывая только major и minor
// компоненты (например "1.22.2" и "1.22" считаются равными, так как major и
// minor совпадают). Возвращает 1, если a > b; -1 если a < b; 0 если равны.
// Используется при резолюции системных рантаймов (go, php): системная версия
// должна быть не ниже требуемой.
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
