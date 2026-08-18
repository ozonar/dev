package toolchain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// phpReleasesURL — URL JSON-индекса релизов PHP.
const phpReleasesURL = "https://www.php.net/releases/?json"

// phpBuilderReleasesURL — базовый URL релизов собранных бинарей PHP.
const phpBuilderReleasesURL = "https://github.com/shivammathur/php-builder/releases/download"

// phpRelease описывает данные о релизе PHP по одной major-версии
// из JSON-индекса php.net.
type phpRelease struct {
	Version           string   `json:"version"`
	SupportedVersions []string `json:"supported_versions"`
}

// distroPrefixes — маппинг комбинаций ID/VERSION_ID из /etc/os-release
// на префикс имени артефакта php-builder (например "ubuntu24.04").
var distroPrefixes = map[string]string{
	"ubuntu22.04": "ubuntu22.04",
	"ubuntu24.04": "ubuntu24.04",
	"ubuntu26.04": "ubuntu26.04",
	"debian11":    "debian11",
	"debian12":    "debian12",
	"debian13":    "debian13",
}

// PhpProgram описывает php-рантайм требуемой версии без обращения к сети.
// URL и тип архива определяются позже, в Manager.Ensure, если нужной версии
// нет локально. Binary известен сразу: в дистрибутиве php-builder бинарь
// лежит по пути usr/bin/php<major.minor>.
func PhpProgram(version string) Program {
	return Program{
		Name:        "php",
		Version:     version,
		Binary:      "usr/bin/php" + version,
		FullCommand: "{php}",
	}
}

// resolvePhp определяет конкретный URL скачивания php для требуемой версии.
// Обращается к php.net (определение актуальной версии) и к php-builder
// (формирование URL). Обновляет версию и путь к бинарю в переданной программе.
func resolvePhp(p Program) (Program, error) {
	majorMinor, err := resolvePhpVersion(p.Version)
	if err != nil {
		return Program{}, err
	}

	distro, err := detectDistro()
	if err != nil {
		return Program{}, err
	}

	archSuffix := ""
	if runtime.GOARCH == "arm64" {
		archSuffix = "_arm64"
	}

	p.Version = majorMinor
	p.Binary = "usr/bin/php" + majorMinor
	p.URL = fmt.Sprintf("%s/%s/php_%s+%s%s.tar.xz", phpBuilderReleasesURL, majorMinor, majorMinor, distro, archSuffix)
	p.Archive = "tar.xz"
	return p, nil
}

// resolvePhpVersion определяет major.minor версию PHP по требованию проекта.
func resolvePhpVersion(required string) (string, error) {
	required = strings.TrimSpace(required)
	if required != "" && required != "latest" {
		v := parseMajorMinor(required)
		return fmt.Sprintf("%d.%d", v[0], v[1]), nil
	}

	supported, err := fetchSupportedPhpVersions()
	if err != nil {
		return "", err
	}
	if len(supported) == 0 {
		return "", fmt.Errorf("no supported PHP versions found")
	}

	return supported[0], nil
}

// fetchSupportedPhpVersions загружает список поддерживаемых версий PHP
// (major.minor) из JSON-индекса php.net и сортирует от новейшей к старейшей.
func fetchSupportedPhpVersions() ([]string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(phpReleasesURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PHP releases: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch PHP releases: HTTP %d", resp.StatusCode)
	}

	var releases map[string]phpRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse PHP releases: %v", err)
	}

	seen := make(map[string]bool)
	var versions []string
	for _, r := range releases {
		for _, v := range r.SupportedVersions {
			if !seen[v] {
				seen[v] = true
				versions = append(versions, v)
			}
		}
	}
	return sortVersionsDesc(versions), nil
}

// sortVersionsDesc сортирует версии вида "major.minor" от новейшей к старейшей.
func sortVersionsDesc(versions []string) []string {
	for i := 1; i < len(versions); i++ {
		for j := i; j > 0 && compareMajorMinor(versions[j], versions[j-1]) > 0; j-- {
			versions[j], versions[j-1] = versions[j-1], versions[j]
		}
	}
	return versions
}

// detectDistro определяет дистрибутив (префикс артефакта php-builder),
// читая /etc/os-release. Например: ubuntu 24.04 -> "ubuntu24.04",
// debian 12 -> "debian12".
func detectDistro() (string, error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", fmt.Errorf("failed to read /etc/os-release: %v", err)
	}

	var id, versionID string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "ID="):
			id = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
		case strings.HasPrefix(line, "VERSION_ID="):
			versionID = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), `"`)
		}
	}

	if id == "" || versionID == "" {
		return "", fmt.Errorf("could not determine OS distribution from /etc/os-release")
	}

	key := id + versionID
	if prefix, ok := distroPrefixes[key]; ok {
		return prefix, nil
	}
	return "", fmt.Errorf("unsupported distribution %s %s", id, versionID)
}
