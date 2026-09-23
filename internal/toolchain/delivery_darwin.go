//go:build darwin

package toolchain

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// darwinDelivery — общие правила доставки macOS, не зависящие от архитектуры.
type darwinDelivery struct{}

func (darwinDelivery) PhpBinaryName(version string) string { return "php" }

// darwinAmd64 — правила доставки для darwin/amd64.
type darwinAmd64 struct{ darwinDelivery }

func (darwinAmd64) PhpSource() phpSource { return darwinPhpSource{arch: "x86_64"} }
func (darwinAmd64) BiomeAsset() string   { return "biome-darwin-x64" }
func (darwinAmd64) RuffTarget() string   { return "x86_64-apple-darwin" }

// darwinArm64 — правила доставки для darwin/arm64.
type darwinArm64 struct{ darwinDelivery }

func (darwinArm64) PhpSource() phpSource { return darwinPhpSource{arch: "aarch64"} }
func (darwinArm64) BiomeAsset() string   { return "biome-darwin-arm64" }
func (darwinArm64) RuffTarget() string   { return "aarch64-apple-darwin" }

func init() {
	registerDelivery("darwin", "amd64", darwinAmd64{})
	registerDelivery("darwin", "arm64", darwinArm64{})
}

// darwinPhpSource — источник PHP через статические сборки static-php-cli
// (macOS): php-builder бинарей для macOS не публикует.
type darwinPhpSource struct {
	// arch — суффикс архитектуры в имени артефакта (x86_64 или aarch64).
	arch string
}

func (s darwinPhpSource) resolve(required string) (resolved, url, archive string, err error) {
	versions, err := fetchStaticPhpVersions(s.arch)
	if err != nil {
		return "", "", "", err
	}

	target := pickStaticPhpVersion(versions, required)
	if target == "" {
		if len(versions) == 0 {
			return "", "", "", fmt.Errorf("no static PHP builds available for macOS %s", s.arch)
		}
		return "", "", "", fmt.Errorf("PHP %s is not available for macOS (available: %s). Install it via Homebrew: brew install php",
			required, summarizeVersions(versions))
	}

	url = fmt.Sprintf("%sphp-%s-cli-macos-%s.tar.gz", phpStaticReleasesURL, target, s.arch)
	return target, url, "tar.gz", nil
}

// fetchStaticPhpVersions загружает листинг статических сборок PHP для macOS
// и возвращает полные версии (major.minor.patch) нужной архитектуры,
// отсортированные от новейшей к старейшей.
func fetchStaticPhpVersions(arch string) ([]string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(phpStaticReleasesURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch static PHP releases: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch static PHP releases: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read static PHP releases: %v", err)
	}

	seen := make(map[string]bool)
	var versions []string
	for _, m := range phpVersionRe.FindAllStringSubmatch(string(body), -1) {
		if m[2] != arch || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		versions = append(versions, m[1])
	}
	return sortFullVersionsDesc(versions), nil
}
