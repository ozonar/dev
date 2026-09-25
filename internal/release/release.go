package release

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"dev/internal/common"
)

// defaultReleasePrefix — префикс имени папки релиза по умолчанию.
const defaultReleasePrefix = "release-"

// defaultReleaseTimeFormat — формат даты-времени в имени папки релиза по умолчанию.
// Все компоненты фиксированной ширины, поэтому лексикографический порядок
// имён совпадает с хронологическим.
const defaultReleaseTimeFormat = "2006-01-02_15-04-05"

// prefix возвращает префикс имени папки релиза: из конфига или дефолтный.
func (r *Release) prefix() string {
	if strings.TrimSpace(r.ReleasePrefix) != "" {
		return r.ReleasePrefix
	}
	return defaultReleasePrefix
}

// timeFormat возвращает формат времени в имени папки релиза.
func (r *Release) timeFormat() string {
	if strings.TrimSpace(r.ReleaseTimeFormat) != "" {
		return r.ReleaseTimeFormat
	}
	return defaultReleaseTimeFormat
}

// NewReleaseName формирует имя папки релиза для заданного момента времени
// с учётом настроек префикса и формата из конфига.
func (r *Release) NewReleaseName(t time.Time) string {
	return r.prefix() + t.Format(r.timeFormat())
}

// ParseReleaseName извлекает время из имени папки релиза по формату конфига.
func (r *Release) ParseReleaseName(name string) (time.Time, bool) {
	if !strings.HasPrefix(name, r.prefix()) {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(r.timeFormat(), strings.TrimPrefix(name, r.prefix()), time.Local)
	return t, err == nil
}

// Prepare копирует содержимое builds_folder в новую папку
// releases_folder/release-<datetime> и возвращает имя созданного релиза.
// Исходная папка сборки при этом не изменяется.
func Prepare(cfg *Release, now time.Time) (string, error) {
	if !common.FileExists(cfg.BuildsFolder) {
		return "", fmt.Errorf("builds folder not found: %s", cfg.BuildsFolder)
	}
	entries, err := os.ReadDir(cfg.BuildsFolder)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("builds folder is empty: %s", cfg.BuildsFolder)
	}
	if err := os.MkdirAll(cfg.ReleasesFolder, 0755); err != nil {
		return "", err
	}

	// При коллизии имени (два prepare за одну секунду) добавляем числовой суффикс.
	base := cfg.NewReleaseName(now)
	dest := filepath.Join(cfg.ReleasesFolder, base)
	for i := 2; common.FileExists(dest); i++ {
		dest = filepath.Join(cfg.ReleasesFolder, fmt.Sprintf("%s-%d", base, i))
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", err
	}
	if err := copyContents(cfg.BuildsFolder, dest); err != nil {
		return "", fmt.Errorf("failed to copy build artifacts: %w", err)
	}
	return filepath.Base(dest), nil
}

// copyContents копирует всё содержимое src в dst, не трогая исходники.
func copyContents(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		from := filepath.Join(src, e.Name())
		to := filepath.Join(dst, e.Name())
		if err := copyRecursive(from, to); err != nil {
			return err
		}
	}
	return nil
}

// copyRecursive рекурсивно копирует файл или директорию src в dst.
func copyRecursive(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyRecursive(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode().Perm())
}

// ReleaseInfo — сведения об одной папке релиза в releases_folder.
type ReleaseInfo struct {
	Name    string    // имя папки, например release-2026-09-22_10-30-00
	Time    time.Time // момент создания релиза
	IsToday bool      // создан сегодня (по локальному времени)
}

// ListReleases возвращает релизы (папки release-*) в порядке от новых к старым.
// Время берётся из имени папки, при невозможности парсинга — из mtime.
func ListReleases(cfg *Release) ([]ReleaseInfo, error) {
	if !common.FileExists(cfg.ReleasesFolder) {
		return nil, fmt.Errorf("releases folder not found: %s", cfg.ReleasesFolder)
	}
	entries, err := os.ReadDir(cfg.ReleasesFolder)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	infos := make([]ReleaseInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), cfg.prefix()) {
			continue
		}
		info := ReleaseInfo{Name: e.Name()}
		if t, ok := cfg.ParseReleaseName(e.Name()); ok {
			info.Time = t
		} else if fi, err := e.Info(); err == nil {
			info.Time = fi.ModTime()
		}
		info.IsToday = sameDay(info.Time, now)
		infos = append(infos, info)
	}
	sort.SliceStable(infos, func(i, j int) bool {
		return infos[i].Time.After(infos[j].Time)
	})
	return infos, nil
}

// sameDay сравнивает две даты по локальному календарному дню.
func sameDay(a, b time.Time) bool {
	ya, ma, da := a.Date()
	yb, mb, db := b.Date()
	return ya == yb && ma == mb && da == db
}

// SwitchRelease переключает симлинк current_release_folder на указанную папку
// релиза внутри releases_folder. Существующий симлинк заменяется, реальные
// директории не трогаются.
func SwitchRelease(cfg *Release, releaseName string) error {
	releaseDir := filepath.Join(cfg.ReleasesFolder, releaseName)
	if !common.FileExists(releaseDir) {
		return fmt.Errorf("release not found: %s", releaseDir)
	}

	link := cfg.CurrentReleaseLink
	if fi, err := os.Lstat(link); err == nil {
		// Защита: заменяем только существующий симлинк, директорию не трогаем.
		if fi.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refusing to replace non-symlink path: %s", link)
		}
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("failed to remove old symlink: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Цель симлинка — абсолютный путь, чтобы ссылка работала из любой директории.
	target, err := filepath.Abs(releaseDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return err
	}
	return os.Symlink(target, link)
}

// CurrentRelease возвращает имя релиза, на который сейчас указывает симлинк
// current_release_folder, и флаг наличия корректной ссылки.
func CurrentRelease(cfg *Release) (string, bool) {
	target, err := os.Readlink(cfg.CurrentReleaseLink)
	if err != nil {
		return "", false
	}
	return filepath.Base(target), true
}

// SelectIndex выводит prompt и читает номер выбора в диапазоне [1..count].
// Пустой ввод означает выбор первого элемента (индекс 0).
func SelectIndex(in io.Reader, out io.Writer, prompt string, count int) (int, error) {
	if count <= 0 {
		return 0, fmt.Errorf("nothing to select")
	}
	fmt.Fprintf(out, "%s [1]: ", prompt)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return 0, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > count {
		return 0, fmt.Errorf("invalid selection %q: expected 1..%d", line, count)
	}
	return n - 1, nil
}
