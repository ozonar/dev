package toolchain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// goRelease описывает один релиз Go из JSON-индекса https://go.dev/dl/?mode=json.
type goRelease struct {
	Version string          `json:"version"`
	Stable  bool            `json:"stable"`
	Files   []goReleaseFile `json:"files"`
}

// goReleaseFile описывает файл дистрибутива в рамках релиза Go.
type goReleaseFile struct {
	Filename string `json:"filename"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Kind     string `json:"kind"`
}

// goDevIndexURL — URL JSON-индекса доступных релизов Go. Параметр include=all
// нужен, чтобы получить полный список версий: без него go.dev отдаёт только
// последние релизы, из-за чего старые требуемые версии (например 1.23) не
// находятся при резолюции.
const goDevIndexURL = "https://go.dev/dl/?mode=json&include=all"

// Go — рантайм Go.
type Go struct {
	runtimeBase
}

// NewGo возвращает go-рантайм требуемой версии без обращения к сети.
func NewGo(version string) Runtime {
	return &Go{runtimeBase{
		name:        "go",
		fullCommand: "{go}",
		binary:      "go/bin/go",
		systemBin:   "go",
		systemVer:   "go version | awk '{print $3}' | sed 's/^go//'",
		version:     version,
	}}
}

// ResolveDownload определяет URL для скачивания go требуемой версии.
func (g *Go) ResolveDownload() (Runtime, error) {
	resolved, url, archive, err := g.resolveDownload()
	if err != nil {
		return nil, err
	}
	ng := NewGo(resolved)
	gg := ng.(*Go)
	gg.url = url
	gg.archive = archive
	return gg, nil
}

// markSystem помечает рантайм как системный с заданным путём к бинарю.
func (g *Go) markSystem(path string) {
	g.binary = path
	g.isSystem = true
}

// resolveDownload определяет конкретный URL скачивания Go для требуемой версии.
// Обращается к go.dev (JSON-индекс релизов), находит подходящую полную версию
// и дистрибутив под текущую платформу. Возвращает фактическую версию, URL
// и тип архива.
func (g *Go) resolveDownload() (resolved, url, archive string, err error) {
	releases, err := fetchGoReleases()
	if err != nil {
		return "", "", "", err
	}

	release, err := findGoRelease(releases, g.version)
	if err != nil {
		return "", "", "", err
	}

	file, err := findGoArchive(release, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", "", "", err
	}

	// Полная версия (например "1.22.12") без префикса "go".
	resolved = strings.TrimPrefix(release.Version, "go")
	return resolved, "https://go.dev/dl/" + file.Filename, "tar.gz", nil
}

// fetchGoReleases загружает и разбирает список релизов Go из JSON-индекса.
func fetchGoReleases() ([]goRelease, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(goDevIndexURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Go releases: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch Go releases: HTTP %d", resp.StatusCode)
	}

	var releases []goRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse Go releases: %v", err)
	}
	return releases, nil
}

// findGoRelease ищет релиз под требуемую версию. Пустая версия или "latest"
// означают новейший стабильный релиз. Индекс отсортирован от новейшей
// версии к старейшей.
func findGoRelease(releases []goRelease, version string) (goRelease, error) {
	version = strings.TrimSpace(version)
	if version == "" || version == "latest" {
		for _, r := range releases {
			if r.Stable {
				return r, nil
			}
		}
		return goRelease{}, fmt.Errorf("no stable Go release found")
	}

	// Требуем префикс "go<major.minor>." для точного совпадения минорной версии.
	prefix := "go" + version + "."
	for _, r := range releases {
		if r.Stable && strings.HasPrefix(r.Version, prefix) {
			return r, nil
		}
	}
	return goRelease{}, fmt.Errorf("Go version %q not found", version)
}

// findGoArchive ищет tar.gz архив релиза для указанных ОС и архитектуры.
func findGoArchive(release goRelease, goos, goarch string) (goReleaseFile, error) {
	for _, f := range release.Files {
		if f.Kind != "archive" {
			continue
		}
		if f.OS != goos || f.Arch != goarch {
			continue
		}
		if !strings.HasSuffix(f.Filename, ".tar.gz") {
			continue
		}
		return f, nil
	}
	return goReleaseFile{}, fmt.Errorf("no Go archive for %s/%s", goos, goarch)
}

// Satisfies определяет, что installed в точности соответствует required по
// major.minor (патч-компонент не влияет: "1.22.12" подходит под требование
// "1.22"). Go требует конкретную минорную версию: системный go 1.23 не
// подходит под требование 1.22, даже если он новее. Пустое требование или
// "latest" подходят любой версии.
func (g *Go) Satisfies(installed, required string) bool {
	required = strings.TrimSpace(required)
	if required == "" || required == "latest" {
		return true
	}
	return compareMajorMinor(installed, required) == 0
}
