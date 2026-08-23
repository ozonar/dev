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

// Php — рантайм PHP.
type Php struct {
	runtimeBase
}

// NewPhp возвращает php-рантайм требуемой версии без обращения к сети.
func NewPhp(version string) Runtime {
	return &Php{runtimeBase{
		name:        "php",
		fullCommand: "{php}",
		binary:      "usr/bin/php" + version,
		systemBin:   "php",
		systemVer:   "php -r 'echo PHP_MAJOR_VERSION.\".\".PHP_MINOR_VERSION;'",
		version:     version,
	}}
}

// ResolveDownload определяет URL для скачивания php требуемой версии.
func (p *Php) ResolveDownload() (Runtime, error) {
	resolved, url, archive, err := p.resolveDownload()
	if err != nil {
		return nil, err
	}
	np := NewPhp(resolved)
	pp := np.(*Php)
	pp.url = url
	pp.archive = archive
	return pp, nil
}

// markSystem помечает рантайм как системный с заданным путём к бинарю.
func (p *Php) markSystem(path string) {
	p.binary = path
	p.isSystem = true
}

// resolveDownload определяет конкретный URL скачивания php для требуемой версии.
// Обращается к php.net (определение актуальной версии) и к php-builder
// (формирование URL). Возвращает фактическую версию, URL и тип архива.
func (p *Php) resolveDownload() (resolved, url, archive string, err error) {
	majorMinor, err := resolvePhpVersion(p.version)
	if err != nil {
		return "", "", "", err
	}

	distro, err := detectDistro()
	if err != nil {
		return "", "", "", err
	}

	archSuffix := ""
	if runtime.GOARCH == "arm64" {
		archSuffix = "_arm64"
	}

	url = fmt.Sprintf("%s/%s/php_%s+%s%s.tar.xz", phpBuilderReleasesURL, majorMinor, majorMinor, distro, archSuffix)
	return majorMinor, url, "tar.xz", nil
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

// Satisfies определяет, что installed минорно старше или равен required,
// но major обязан совпадать. Например php 8.3 подходит под требование 8.2,
// а php 9.0 — нет. Пустое требование или "latest" подходят любой версии.
func (p *Php) Satisfies(installed, required string) bool {
	required = strings.TrimSpace(required)
	if required == "" || required == "latest" {
		return true
	}
	a := parseMajorMinor(installed)
	b := parseMajorMinor(required)
	return a[0] == b[0] && a[1] >= b[1]
}
