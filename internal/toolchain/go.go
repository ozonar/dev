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

// goDevIndexURL — URL JSON-индекса доступных релизов Go.
const goDevIndexURL = "https://go.dev/dl/?mode=json"

// GoProgram описывает Go-тулчейн требуемой версии без обращения к сети.
// URL и полная версия определяются позже, в Manager.Ensure, если нужной
// версии нет локально. Binary известен сразу: в дистрибутиве Go бинарь лежит
// по пути go/bin/go.
func GoProgram(version string) Program {
	return Program{
		Name:        "go",
		Version:     version,
		Binary:      "go/bin/go",
		FullCommand: "{go}",
	}
}

// resolveGo определяет конкретный URL скачивания Go для требуемой версии.
// Обращается к go.dev (JSON-индекс релизов), находит подходящую полную версию
// и дистрибутив под текущую платформу. Обновляет версию и URL в программе.
func resolveGo(p Program) (Program, error) {
	releases, err := fetchGoReleases()
	if err != nil {
		return Program{}, err
	}

	release, err := findGoRelease(releases, p.Version)
	if err != nil {
		return Program{}, err
	}

	file, err := findGoArchive(release, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return Program{}, err
	}

	// Полная версия (например "1.22.12") без префикса "go".
	p.Version = strings.TrimPrefix(release.Version, "go")
	p.URL = "https://go.dev/dl/" + file.Filename
	p.Archive = "tar.gz"
	return p, nil
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
