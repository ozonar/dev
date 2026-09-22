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
