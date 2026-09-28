package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig создаёт release.yml с указанным содержимым в директории dir.
func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadConfigOK проверяет чтение валидного конфига с несколькими релизами.
func TestLoadConfigOK(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
releases:
  backend:
    builds_folder: ./builds/backend
    releases_folder: ./releases/backend
    current_release_folder: ./current/backend
  frontend:
    builds_folder: ./builds/frontend
    releases_folder: ./releases/frontend
    current_release_folder: ./current/frontend
`)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig вернула ошибку: %v", err)
	}
	if len(cfg.Releases) != 2 {
		t.Fatalf("ожидалось 2 релиза, получено %d", len(cfg.Releases))
	}
	rel := cfg.Releases["backend"]
	if rel == nil {
		t.Fatal("релиз backend отсутствует")
	}
	if rel.BuildsFolder != "./builds/backend" ||
		rel.ReleasesFolder != "./releases/backend" ||
		rel.CurrentReleaseLink != "./current/backend" {
		t.Fatalf("неожиданные поля релиза: %+v", rel)
	}
}

// TestLoadConfigMissingFile проверяет ошибку при отсутствии release.yml.
func TestLoadConfigMissingFile(t *testing.T) {
	if _, err := LoadConfig(t.TempDir()); err == nil {
		t.Fatal("ожидалась ошибка при отсутствии release.yml")
	}
}

// TestLoadConfigInvalidYAML проверяет ошибку при битом YAML.
func TestLoadConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "releases: [unclosed\n")
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("ожидалась ошибка при невалидном YAML")
	}
}

// TestLoadConfigNoReleases проверяет ошибку при пустом списке релизов.
func TestLoadConfigNoReleases(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "releases: {}\n")
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("ожидалась ошибка при пустом списке релизов")
	}
}

// TestLoadConfigNullRelease проверяет ошибку, когда у релиза нет тела (null).
func TestLoadConfigNullRelease(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "releases:\n  backend:\n")
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("ожидалась ошибка при null-релизе")
	}
}

// TestLoadConfigMissingFields проверяет ошибку при неполном наборе путей.
func TestLoadConfigMissingFields(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
releases:
  backend:
    builds_folder: ./builds
`)
	_, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("ожидалась ошибка при неполном наборе путей")
	}
	if !strings.Contains(err.Error(), "backend") {
		t.Fatalf("ошибка должна упоминать имя релиза: %v", err)
	}
}

// TestLoadConfigInvalidTimeFormat проверяет отказ при невалидном формате времени.
func TestLoadConfigInvalidTimeFormat(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
releases:
  backend:
    builds_folder: ./builds
    releases_folder: ./releases
    current_release_folder: ./current
    release_time_format: "2006-13-99"
`)
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("ожидалась ошибка при невалидном release_time_format")
	}
}

// TestLoadConfigGroups проверяет чтение групп и флага important у релизов.
func TestLoadConfigGroups(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
releases:
  backend:
    builds_folder: ./builds/backend
    releases_folder: ./releases/backend
    current_release_folder: ./current/backend
    groups: [web, api]
  frontend:
    builds_folder: ./builds/frontend
    releases_folder: ./releases/frontend
    current_release_folder: ./current/frontend
    groups: [web]
    important: true
`)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig вернула ошибку: %v", err)
	}
	rel := cfg.Releases["backend"]
	if len(rel.Groups) != 2 || rel.Groups[0] != "web" || rel.Groups[1] != "api" {
		t.Fatalf("неожиданные группы релиза backend: %v", rel.Groups)
	}
	if !cfg.Releases["frontend"].Important {
		t.Fatal("релиз frontend должен быть помечен important")
	}
	if cfg.Releases["backend"].Important {
		t.Fatal("релиз backend не должен быть помечен important")
	}
}

// TestLoadConfigDuplicateGroup проверяет отказ при дубликате имени группы.
func TestLoadConfigDuplicateGroup(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
releases:
  backend:
    builds_folder: ./builds
    releases_folder: ./releases
    current_release_folder: ./current
    groups: [web, web]
`)
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("ожидалась ошибка при дубликате имени группы")
	}
}

// TestLoadConfigEmptyGroupName проверяет отказ при пустом имени группы.
func TestLoadConfigEmptyGroupName(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
releases:
  backend:
    builds_folder: ./builds
    releases_folder: ./releases
    current_release_folder: ./current
    groups: ["", web]
`)
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("ожидалась ошибка при пустом имени группы")
	}
}

// newTestConfig создаёт конфиг с тремя релизами: backend и frontend входят
// в группу web, frontend помечен important; mobile вне групп.
func newTestConfig(t *testing.T) *Config {
	t.Helper()
	base := t.TempDir()
	mkRelease := func(name string, groups []string, important bool) *Release {
		builds := filepath.Join(base, "builds-"+name)
		if err := os.MkdirAll(builds, 0755); err != nil {
			t.Fatal(err)
		}
		return &Release{
			BuildsFolder:       builds,
			ReleasesFolder:     filepath.Join(base, "releases-"+name),
			CurrentReleaseLink: filepath.Join(base, "current-"+name),
			Groups:             groups,
			Important:          important,
		}
	}
	return &Config{
		Releases: map[string]*Release{
			"backend":  mkRelease("backend", []string{"web"}, false),
			"frontend": mkRelease("frontend", []string{"web"}, true),
			"mobile":   mkRelease("mobile", nil, false),
		},
	}
}

// TestGroupNames проверяет вычисление имён групп из свойств релизов.
func TestGroupNames(t *testing.T) {
	cfg := newTestConfig(t)
	names := cfg.GroupNames()
	if len(names) != 1 || names[0] != "web" {
		t.Fatalf("ожидалась группа web, получено %v", names)
	}
}

// TestGroupMembers проверяет состав группы и ошибки для неизвестных имён.
func TestGroupMembers(t *testing.T) {
	cfg := newTestConfig(t)
	members, err := cfg.GroupMembers("web")
	if err != nil {
		t.Fatalf("GroupMembers вернула ошибку: %v", err)
	}
	if len(members) != 2 || members[0] != "backend" || members[1] != "frontend" {
		t.Fatalf("неожиданный состав группы web: %v", members)
	}
	if _, err := cfg.GroupMembers("nope"); err == nil {
		t.Fatal("ожидалась ошибка для неизвестной группы")
	}
	if _, err := cfg.GroupMembers("  "); err == nil {
		t.Fatal("ожидалась ошибка для пустого имени группы")
	}
}

// TestGroupsOf проверяет принадлежность релиза к группам.
func TestGroupsOf(t *testing.T) {
	cfg := newTestConfig(t)
	groups := cfg.GroupsOf("backend")
	if len(groups) != 1 || groups[0] != "web" {
		t.Fatalf("неожиданные группы backend: %v", groups)
	}
	if len(cfg.GroupsOf("mobile")) != 0 {
		t.Fatal("у mobile не должно быть групп")
	}
	if cfg.GroupsOf("unknown") != nil {
		t.Fatal("для неизвестного релиза GroupsOf должна вернуть nil")
	}
}

// TestResolveTarget проверяет разрешение имён в цели релиза или группы.
func TestResolveTarget(t *testing.T) {
	cfg := newTestConfig(t)
	target, err := cfg.ResolveTarget("backend")
	if err != nil || target.Kind != TargetRelease || target.Name != "backend" {
		t.Fatalf("ожидался релиз backend, получено %+v err=%v", target, err)
	}
	target, err = cfg.ResolveTarget("web")
	if err != nil || target.Kind != TargetGroup || target.Name != "web" {
		t.Fatalf("ожидалась группа web, получено %+v err=%v", target, err)
	}
	if _, err := cfg.ResolveTarget("nope"); err == nil {
		t.Fatal("ожидалась ошибка для неизвестного имени")
	}
	if _, err := cfg.ResolveTarget(""); err == nil {
		t.Fatal("ожидалась ошибка для пустого имени")
	}
}

// TestResolveTargetReleaseWins проверяет приоритет релиза над группой
// при совпадении имён.
func TestResolveTargetReleaseWins(t *testing.T) {
	cfg := newTestConfig(t)
	// Добавляем группу с именем существующего релиза.
	cfg.Releases["backend"].Groups = append(cfg.Releases["backend"].Groups, "mobile")
	target, err := cfg.ResolveTarget("mobile")
	if err != nil {
		t.Fatalf("ResolveTarget вернула ошибку: %v", err)
	}
	if target.Kind != TargetRelease || target.Name != "mobile" {
		t.Fatalf("ожидался приоритет релиза mobile, получено %+v", target)
	}
}

// TestAllTargetsOrder проверяет порядок целей: сначала группы, затем релизы.
func TestAllTargetsOrder(t *testing.T) {
	cfg := newTestConfig(t)
	targets := cfg.AllTargets()
	if len(targets) != 4 {
		t.Fatalf("ожидалось 4 цели, получено %d", len(targets))
	}
	if targets[0].Kind != TargetGroup || targets[0].Name != "web" {
		t.Fatalf("первой должна идти группа web, получено %+v", targets[0])
	}
	for i, want := range []string{"backend", "frontend", "mobile"} {
		got := targets[i+1]
		if got.Kind != TargetRelease || got.Name != want {
			t.Fatalf("ожидался релиз %s, получено %+v", want, got)
		}
	}
}

// TestNeedsConfirmation проверяет необходимость переспроса на шаге switch.
func TestNeedsConfirmation(t *testing.T) {
	cfg := newTestConfig(t)
	// Важный релиз требует подтверждения.
	if !cfg.NeedsConfirmation(Target{Kind: TargetRelease, Name: "frontend"}) {
		t.Fatal("важный релиз должен требовать подтверждения")
	}
	// Обычный релиз — нет.
	if cfg.NeedsConfirmation(Target{Kind: TargetRelease, Name: "backend"}) {
		t.Fatal("обычный релиз не должен требовать подтверждения")
	}
	// Группа с важным релизом требует подтверждения.
	if !cfg.NeedsConfirmation(Target{Kind: TargetGroup, Name: "web"}) {
		t.Fatal("группа с важным релизом должна требовать подтверждения")
	}
	// Неизвестная цель подтверждения не требует.
	if cfg.NeedsConfirmation(Target{Kind: TargetGroup, Name: "nope"}) {
		t.Fatal("неизвестная группа не должна требовать подтверждения")
	}
}

// TestTargetLabel проверяет человекочитаемое описание цели.
func TestTargetLabel(t *testing.T) {
	cfg := newTestConfig(t)
	label := cfg.TargetLabel(Target{Kind: TargetGroup, Name: "web"})
	if !strings.Contains(label, "web") || !strings.Contains(label, "backend") {
		t.Fatalf("неожиданная метка группы: %q", label)
	}
	label = cfg.TargetLabel(Target{Kind: TargetRelease, Name: "backend"})
	if !strings.Contains(label, "backend") {
		t.Fatalf("неожиданная метка релиза: %q", label)
	}
}
