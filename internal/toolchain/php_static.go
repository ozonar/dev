// Общие данные и чистые функции для статических сборок PHP (static-php-cli),
// используемых на macOS. Сетевая часть (fetchStaticPhpVersions) живёт в
// delivery_darwin.go, чтобы не тянуть сеть на других платформах.
package toolchain

import (
	"strings"
)

// pickStaticPhpVersion выбирает из списка versions (отсортирован от новейшей
// к старейшей) самую свежую версию, удовлетворяющую требованию required
// (major.minor). Пустое требование или "latest" берут самую свежую из всех.
// Возвращает "" если подходящей версии нет.
func pickStaticPhpVersion(versions []string, required string) string {
	if !IsLatestRequirement(required) {
		prefix := strings.TrimSpace(required) + "."
		for _, v := range versions {
			if strings.HasPrefix(v, prefix) {
				return v
			}
		}
		return ""
	}
	if len(versions) == 0 {
		return ""
	}
	return versions[0]
}

// summarizeVersions возвращает компактное описание списка версий для сообщений
// об ошибках (до 5 первых версий).
func summarizeVersions(versions []string) string {
	const limit = 5
	if len(versions) <= limit {
		return strings.Join(versions, ", ")
	}
	return strings.Join(versions[:limit], ", ") + ", ..."
}
