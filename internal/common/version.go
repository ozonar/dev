package common

import (
	"strconv"
	"strings"
)

// NumericParts возвращает числовые компоненты версии, разделённой точками.
// Нечисловые компоненты пропускаются, поэтому корректно разбираются как
// полные версии ("1.22.12"), так и версии с префиксом ("go1.22").
func NumericParts(v string) []int {
	var parts []int
	for _, part := range strings.Split(v, ".") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		parts = append(parts, n)
	}
	return parts
}
