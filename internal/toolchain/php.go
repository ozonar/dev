package toolchain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// phpReleasesURL — URL JSON-индекса релизов PHP.
const phpReleasesURL = "https://www.php.net/releases/?json"

// phpRelease описывает данные о релизе PHP по одной major-версии
// из JSON-индекса php.net.
type phpRelease struct {
	Version           string   `json:"version"`
	SupportedVersions []string `json:"supported_versions"`
}

// Php — рантайм PHP.
type Php struct {
	runtimeBase
}

// NewPhp возвращает php-рантайм требуемой версии без обращения к сети.
// Имя бинаря внутри архива зависит от платформы — его определяет адаптер
// доставки (см. Delivery.PhpBinaryName).
func NewPhp(version string) Runtime {
	return &Php{runtimeBase{
		name:        "php",
		fullCommand: "{php}",
		binary:      CurrentDelivery().PhpBinaryName(version),
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

// resolveDownload определяет конкретный URL скачивания php для требуемой
// версии. Выбор источника (php-builder на Linux, static-php-cli на macOS)
// делегируется адаптеру доставки — здесь нет ветвлений по платформе.
// Возвращает фактическую версию, URL и тип архива.
func (p *Php) resolveDownload() (resolved, url, archive string, err error) {
	majorMinor, err := resolvePhpVersion(p.version)
	if err != nil {
		return "", "", "", err
	}
	return CurrentDelivery().PhpSource().resolve(majorMinor)
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
