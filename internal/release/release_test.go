package release

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestNewReleaseNameRoundTrip проверяет формирование и парсинг имени релиза.
func TestNewReleaseNameRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 22, 10, 30, 5, 0, time.Local)
	rel := &Release{}
	name := rel.NewReleaseName(now)
	if name != "release-2026-09-22_10-30-05" {
		t.Fatalf("неожиданное имя релиза: %s", name)
	}
	parsed, ok := rel.ParseReleaseName(name)
	if !ok || !parsed.Equal(now) {
		t.Fatalf("парсинг не удался: %v ok=%v", parsed, ok)
	}
}

// TestParseReleaseNameInvalid проверяет отказ парсинга для посторонних имён.
func TestParseReleaseNameInvalid(t *testing.T) {
	rel := &Release{}
	if _, ok := rel.ParseReleaseName("some-dir"); ok {
		t.Fatal("ожидался false для имени без префикса release-")
	}
	if _, ok := rel.ParseReleaseName("release-not-a-date"); ok {
		t.Fatal("ожидался false для некорректной даты")
	}
}

// TestCustomPrefixAndFormat проверяет проброс необязательных параметров
// release_prefix и release_time_format из конфига в имя папки релиза.
func TestCustomPrefixAndFormat(t *testing.T) {
	rel := &Release{
		ReleasePrefix:     "build-",
		ReleaseTimeFormat: "2006-01-02",
	}
	now := time.Date(2026, 9, 22, 10, 30, 5, 0, time.Local)
	name := rel.NewReleaseName(now)
	if name != "build-2026-09-22" {
		t.Fatalf("неожиданное имя релиза: %s", name)
	}
	parsed, ok := rel.ParseReleaseName(name)
	if !ok {
		t.Fatal("парсинг кастомного имени не удался")
	}
	if parsed.Day() != 22 || parsed.Month() != time.September {
		t.Fatalf("неожиданная дата: %v", parsed)
	}
}

// TestPrepareCopy проверяет копирование содержимого builds_folder в новую
// папку релиза: исходники остаются на месте, копия появляется в releases_folder.
func TestPrepareCopy(t *testing.T) {
	base := t.TempDir()
	builds := filepath.Join(base, "builds")
	releases := filepath.Join(base, "releases")
	if err := os.MkdirAll(filepath.Join(builds, "app"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(builds, "index.html"), []byte("<html></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &Release{BuildsFolder: builds, ReleasesFolder: releases}
	now := time.Date(2026, 9, 22, 8, 0, 0, 0, time.Local)

	name, err := Prepare(cfg, now)
	if err != nil {
		t.Fatalf("Prepare вернула ошибку: %v", err)
	}
	if name != "release-2026-09-22_08-00-00" {
		t.Fatalf("неожиданное имя релиза: %s", name)
	}

	// Содержимое скопировано в папку релиза.
	dst := filepath.Join(releases, name)
	if _, err := os.Stat(filepath.Join(dst, "index.html")); err != nil {
		t.Fatal("index.html не скопирован")
	}
	if _, err := os.Stat(filepath.Join(dst, "app")); err != nil {
		t.Fatal("директория app не скопирована")
	}

	// builds_folder остался нетронутым после копирования.
	entries, err := os.ReadDir(builds)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("builds_folder должна содержать 2 элемента, найдено %d", len(entries))
	}
	if _, err := os.Stat(filepath.Join(builds, "index.html")); err != nil {
		t.Fatal("index.html не должен удаляться из builds_folder")
	}
	if _, err := os.Stat(filepath.Join(builds, "app")); err != nil {
		t.Fatal("директория app не должна удаляться из builds_folder")
	}
}

// TestPrepareMissingBuilds проверяет ошибку при отсутствии папки сборки.
func TestPrepareMissingBuilds(t *testing.T) {
	cfg := &Release{BuildsFolder: filepath.Join(t.TempDir(), "nope")}
	if _, err := Prepare(cfg, time.Now()); err == nil {
		t.Fatal("ожидалась ошибка при отсутствии builds_folder")
	}
}

// TestPrepareEmptyBuilds проверяет ошибку при пустой папке сборки.
func TestPrepareEmptyBuilds(t *testing.T) {
	base := t.TempDir()
	builds := filepath.Join(base, "builds")
	if err := os.MkdirAll(builds, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &Release{BuildsFolder: builds}
	if _, err := Prepare(cfg, time.Now()); err == nil {
		t.Fatal("ожидалась ошибка при пустой builds_folder")
	}
}

// TestPrepareCollision проверяет добавление числового суффикса при коллизии имени.
func TestPrepareCollision(t *testing.T) {
	base := t.TempDir()
	builds := filepath.Join(base, "builds")
	releases := filepath.Join(base, "releases")
	if err := os.MkdirAll(builds, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(builds, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	// Заранее создаём папку с тем же именем.
	now := time.Date(2026, 9, 22, 8, 0, 0, 0, time.Local)
	if err := os.MkdirAll(filepath.Join(releases, (&Release{}).NewReleaseName(now)), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &Release{BuildsFolder: builds, ReleasesFolder: releases}
	name, err := Prepare(cfg, now)
	if err != nil {
		t.Fatalf("Prepare вернула ошибку: %v", err)
	}
	if name != "release-2026-09-22_08-00-00-2" {
		t.Fatalf("ожидалось имя с суффиксом, получено %s", name)
	}
	if _, err := os.Stat(filepath.Join(releases, name, "a.txt")); err != nil {
		t.Fatal("файл не скопирован в суффиксную папку")
	}
}

// TestListReleases проверяет порядок от новых к старым, фильтрацию
// посторонних папок и признак «сегодня».
func TestListReleases(t *testing.T) {
	base := t.TempDir()
	releases := filepath.Join(base, "releases")
	now := time.Now()
	rel := &Release{}
	names := []string{
		rel.NewReleaseName(now.AddDate(0, 0, -2)),
		rel.NewReleaseName(now.AddDate(0, 0, -1)),
		rel.NewReleaseName(now),
	}
	for _, d := range names {
		if err := os.MkdirAll(filepath.Join(releases, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Посторонняя папка должна игнорироваться.
	if err := os.MkdirAll(filepath.Join(releases, "other-dir"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &Release{ReleasesFolder: releases}
	infos, err := ListReleases(cfg)
	if err != nil {
		t.Fatalf("ListReleases вернула ошибку: %v", err)
	}
	if len(infos) != 3 {
		t.Fatalf("ожидалось 3 релиза, получено %d", len(infos))
	}
	if infos[0].Name != names[2] {
		t.Fatalf("самый новый должен быть первым: %s != %s", infos[0].Name, names[2])
	}
	if infos[2].Name != names[0] {
		t.Fatalf("самый старый должен быть последним: %s != %s", infos[2].Name, names[0])
	}
	if !infos[0].IsToday {
		t.Fatal("самый новый релиз должен быть помечен как сегодняшний")
	}
	if infos[1].IsToday || infos[2].IsToday {
		t.Fatal("старые релизы не должны быть помечены как сегодняшние")
	}
}

// TestListReleasesMissingFolder проверяет ошибку при отсутствии releases_folder.
func TestListReleasesMissingFolder(t *testing.T) {
	cfg := &Release{ReleasesFolder: filepath.Join(t.TempDir(), "nope")}
	if _, err := ListReleases(cfg); err == nil {
		t.Fatal("ожидалась ошибка при отсутствии releases_folder")
	}
}

// TestSwitchRelease проверяет создание и повторное переключение симлинка.
func TestSwitchRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("создание симлинков на Windows требует привилегий")
	}
	base := t.TempDir()
	releases := filepath.Join(base, "releases")
	rel1 := filepath.Join(releases, "release-2026-09-22_10-00-00")
	if err := os.MkdirAll(rel1, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "current")
	cfg := &Release{ReleasesFolder: releases, CurrentReleaseLink: link}

	if err := SwitchRelease(cfg, "release-2026-09-22_10-00-00"); err != nil {
		t.Fatalf("SwitchRelease вернула ошибку: %v", err)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink вернула ошибку: %v", err)
	}
	abs, _ := filepath.Abs(rel1)
	if target != abs {
		t.Fatalf("ожидалась цель %s, получена %s", abs, target)
	}

	// Повторное переключение на другой релиз.
	rel2 := filepath.Join(releases, "release-2026-09-23_10-00-00")
	if err := os.MkdirAll(rel2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := SwitchRelease(cfg, "release-2026-09-23_10-00-00"); err != nil {
		t.Fatalf("повторный SwitchRelease вернул ошибку: %v", err)
	}
	target2, _ := os.Readlink(link)
	abs2, _ := filepath.Abs(rel2)
	if target2 != abs2 {
		t.Fatalf("ожидалась цель %s, получена %s", abs2, target2)
	}
}

// TestSwitchReleaseMissing проверяет ошибку при несуществующем релизе.
func TestSwitchReleaseMissing(t *testing.T) {
	cfg := &Release{
		ReleasesFolder:     filepath.Join(t.TempDir(), "releases"),
		CurrentReleaseLink: filepath.Join(t.TempDir(), "current"),
	}
	if err := SwitchRelease(cfg, "release-2026-09-22_10-00-00"); err == nil {
		t.Fatal("ожидалась ошибка при переключении на несуществующий релиз")
	}
}

// TestSwitchReleaseRefusesDir проверяет отказ заменять реальную директорию.
func TestSwitchReleaseRefusesDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("создание симлинков на Windows требует привилегий")
	}
	base := t.TempDir()
	releases := filepath.Join(base, "releases")
	if err := os.MkdirAll(filepath.Join(releases, "release-2026-09-22_10-00-00"), 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "current")
	if err := os.MkdirAll(link, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &Release{ReleasesFolder: releases, CurrentReleaseLink: link}
	if err := SwitchRelease(cfg, "release-2026-09-22_10-00-00"); err == nil {
		t.Fatal("ожидалась ошибка при замене реальной директории")
	}
}

// TestCurrentRelease проверяет определение текущего релиза по симлинку.
func TestCurrentRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("создание симлинков на Windows требует привилегий")
	}
	base := t.TempDir()
	releases := filepath.Join(base, "releases")
	relDir := filepath.Join(releases, "release-2026-09-22_10-00-00")
	if err := os.MkdirAll(relDir, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "current")
	cfg := &Release{ReleasesFolder: releases, CurrentReleaseLink: link}

	// Симлинка ещё нет — текущего релиза нет.
	if name, ok := CurrentRelease(cfg); ok {
		t.Fatalf("неожиданный текущий релиз до переключения: %s", name)
	}
	if err := SwitchRelease(cfg, "release-2026-09-22_10-00-00"); err != nil {
		t.Fatalf("SwitchRelease вернула ошибку: %v", err)
	}
	name, ok := CurrentRelease(cfg)
	if !ok || name != "release-2026-09-22_10-00-00" {
		t.Fatalf("ожидался текущий релиз release-2026-09-22_10-00-00, получен %q ok=%v", name, ok)
	}
}

// TestSelectIndexDefault проверяет выбор первого элемента по умолчанию.
func TestSelectIndexDefault(t *testing.T) {
	var out bytes.Buffer
	idx, err := SelectIndex(strings.NewReader("\n"), &out, "Select", 3)
	if err != nil {
		t.Fatalf("SelectIndex вернула ошибку: %v", err)
	}
	if idx != 0 {
		t.Fatalf("ожидался индекс 0 по умолчанию, получен %d", idx)
	}
	if !strings.Contains(out.String(), "[1]") {
		t.Fatalf("prompt не содержит подсказки: %q", out.String())
	}
}

// TestSelectIndexExplicit проверяет явный выбор номера.
func TestSelectIndexExplicit(t *testing.T) {
	idx, err := SelectIndex(strings.NewReader("3\n"), &bytes.Buffer{}, "Select", 3)
	if err != nil {
		t.Fatalf("SelectIndex вернула ошибку: %v", err)
	}
	if idx != 2 {
		t.Fatalf("ожидался индекс 2, получен %d", idx)
	}
}

// TestSelectIndexInvalid проверяет ошибки при некорректном вводе.
func TestSelectIndexInvalid(t *testing.T) {
	if _, err := SelectIndex(strings.NewReader("7\n"), &bytes.Buffer{}, "Select", 3); err == nil {
		t.Fatal("ожидалась ошибка для номера вне диапазона")
	}
	if _, err := SelectIndex(strings.NewReader("abc\n"), &bytes.Buffer{}, "Select", 3); err == nil {
		t.Fatal("ожидалась ошибка для нечислового ввода")
	}
}

// TestSelectIndexEmptyList проверяет ошибку при пустом списке выбора.
func TestSelectIndexEmptyList(t *testing.T) {
	if _, err := SelectIndex(strings.NewReader("\n"), &bytes.Buffer{}, "Select", 0); err == nil {
		t.Fatal("ожидалась ошибка при пустом списке выбора")
	}
}

// TestPrepareCustomPrefix проверяет Prepare и ListReleases с кастомными
// префиксом и форматом времени.
func TestPrepareCustomPrefix(t *testing.T) {
	base := t.TempDir()
	builds := filepath.Join(base, "builds")
	releases := filepath.Join(base, "releases")
	if err := os.MkdirAll(builds, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(builds, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &Release{
		BuildsFolder:      builds,
		ReleasesFolder:    releases,
		ReleasePrefix:     "build-",
		ReleaseTimeFormat: "2006-01-02",
	}

	name, err := Prepare(cfg, time.Date(2026, 9, 22, 8, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("Prepare вернула ошибку: %v", err)
	}
	if name != "build-2026-09-22" {
		t.Fatalf("неожиданное имя релиза: %s", name)
	}
	if _, err := os.Stat(filepath.Join(releases, name, "a.txt")); err != nil {
		t.Fatal("файл не скопирован")
	}

	// ListReleases должен распознать кастомный префикс.
	infos, err := ListReleases(cfg)
	if err != nil {
		t.Fatalf("ListReleases вернула ошибку: %v", err)
	}
	if len(infos) != 1 || infos[0].Name != name {
		t.Fatalf("неожиданный список релизов: %+v", infos)
	}
}
