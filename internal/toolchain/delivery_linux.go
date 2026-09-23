//go:build linux

package toolchain

import (
	"fmt"
	"os"
	"strings"
)

// linuxDelivery — общие правила доставки Linux, не зависящие от архитектуры.
type linuxDelivery struct{}

func (linuxDelivery) PhpBinaryName(version string) string { return "usr/bin/php" + version }

// linuxAmd64 — правила доставки для linux/amd64.
type linuxAmd64 struct{ linuxDelivery }

func (linuxAmd64) PhpSource() phpSource { return linuxPhpSource{archSuffix: ""} }
func (linuxAmd64) BiomeAsset() string   { return "biome-linux-x64" }
func (linuxAmd64) RuffTarget() string   { return "x86_64-unknown-linux-gnu" }

// linuxArm64 — правила доставки для linux/arm64.
type linuxArm64 struct{ linuxDelivery }

func (linuxArm64) PhpSource() phpSource { return linuxPhpSource{archSuffix: "_arm64"} }
func (linuxArm64) BiomeAsset() string   { return "biome-linux-arm64" }
func (linuxArm64) RuffTarget() string   { return "aarch64-unknown-linux-gnu" }

func init() {
	registerDelivery("linux", "amd64", linuxAmd64{})
	registerDelivery("linux", "arm64", linuxArm64{})
}

// phpBuilderReleasesURL — базовый URL релизов собранных бинарей PHP.
const phpBuilderReleasesURL = "https://github.com/shivammathur/php-builder/releases/download"

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

// linuxPhpSource — источник PHP через сборки php-builder (Linux).
type linuxPhpSource struct {
	// archSuffix — суффикс архитектуры в имени артефакта ("_arm64" или "").
	archSuffix string
}

func (s linuxPhpSource) resolve(required string) (resolved, url, archive string, err error) {
	distro, err := detectDistro()
	if err != nil {
		return "", "", "", err
	}

	url = fmt.Sprintf("%s/%s/php_%s+%s%s.tar.xz", phpBuilderReleasesURL, required, required, distro, s.archSuffix)
	return required, url, "tar.xz", nil
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
