// Общие данные и чистые функции для статических сборок PHP (static-php-cli),
// используемых на macOS. Сетевая часть (fetchStaticPhpVersions) живёт в
// delivery_darwin.go, чтобы не тянуть сеть на других платформах.
package toolchain

import (
	"regexp"
	"strings"
)

// phpStaticReleasesURL — листинг статических сборок PHP (static-php-cli).
const phpStaticReleasesURL = "https://dl.static-php.dev/static-php-cli/common/"

// phpVersionRe — шаблон имени файла статической сборки PHP для macOS,
// например "php-8.4.23-cli-macos-aarch64.tar.gz".
var phpVersionRe = regexp.MustCompile(`php-(\d+\.\d+\.\d+)-cli-macos-(x86_64|aarch64)\.tar\.gz`)

// pickStaticPhpVersion выбирает из списка versions (отсортирован от новейшей
// к старейшей) самую свежую версию, удовлетворяющую требованию required
// (major.minor). Пустое требование или "latest" берут самую свежую из всех.
// Возвращает "" если подходящей версии нет.
func pickStaticPhpVersion(versions []string, required string) string {
	required = strings.TrimSpace(required)
	if required != "" && required != "latest" {
		prefix := required + "."
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
