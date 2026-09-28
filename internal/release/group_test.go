package release

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// prepareGroupConfig создаёт конфиг с двумя релизами группы web, у каждого
// своя непустая папка сборки.
func prepareGroupConfig(t *testing.T) *Config {
	t.Helper()
	base := t.TempDir()
	mkRelease := func(name string) *Release {
		builds := filepath.Join(base, "builds-"+name)
		if err := os.MkdirAll(builds, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(builds, name+".txt"), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
		return &Release{
			BuildsFolder:       builds,
			ReleasesFolder:     filepath.Join(base, "releases-"+name),
			CurrentReleaseLink: filepath.Join(base, "current-"+name),
			Groups:             []string{"web"},
		}
	}
	return &Config{
		Releases: map[string]*Release{
			"backend":  mkRelease("backend"),
			"frontend": mkRelease("frontend"),
		},
	}
}

// TestPrepareGroup проверяет подготовку всех релизов группы в алфавитном
// порядке имён с копированием содержимого.
func TestPrepareGroup(t *testing.T) {
	cfg := prepareGroupConfig(t)
	now := time.Date(2026, 9, 22, 8, 0, 0, 0, time.Local)

	prepared, err := cfg.PrepareGroup("web", now)
	if err != nil {
		t.Fatalf("PrepareGroup вернула ошибку: %v", err)
	}
	if len(prepared) != 2 {
		t.Fatalf("ожидалось 2 подготовленных релиза, получено %d", len(prepared))
	}
	// Имена созданных папок отличаются для разных релизов (разные releases_folder).
	for _, p := range prepared {
		if p.Release == "" || p.Name == "" {
			t.Fatalf("неполный результат: %+v", p)
		}
		dst := filepath.Join(cfg.Releases[p.Release].ReleasesFolder, p.Name)
		if _, err := os.Stat(filepath.Join(dst, p.Release+".txt")); err != nil {
			t.Fatalf("файл релиза %s не скопирован: %v", p.Release, err)
		}
	}
}

// TestPrepareGroupOrder проверяет детерминированный порядок группы
// независимо от порядка перечисления в мапе.
func TestPrepareGroupOrder(t *testing.T) {
	cfg := prepareGroupConfig(t)
	now := time.Date(2026, 9, 22, 8, 0, 0, 0, time.Local)

	prepared, err := cfg.PrepareGroup("web", now)
	if err != nil {
		t.Fatalf("PrepareGroup вернула ошибку: %v", err)
	}
	if prepared[0].Release != "backend" || prepared[1].Release != "frontend" {
		t.Fatalf("ожидался порядок backend, frontend, получено %+v", prepared)
	}
}

// TestPrepareGroupUnknown проверяет ошибку для неизвестной группы.
func TestPrepareGroupUnknown(t *testing.T) {
	cfg := prepareGroupConfig(t)
	if _, err := cfg.PrepareGroup("nope", time.Now()); err == nil {
		t.Fatal("ожидалась ошибка для неизвестной группы")
	}
}

// TestPrepareGroupPartialFailure проверяет остановку при ошибке: первый
// релиз подготовлен, второй не может быть собран (нет builds_folder).
func TestPrepareGroupPartialFailure(t *testing.T) {
	cfg := prepareGroupConfig(t)
	// Убираем папку сборки у frontend (второй по алфавиту).
	cfg.Releases["frontend"].BuildsFolder = filepath.Join(t.TempDir(), "missing-builds")

	prepared, err := cfg.PrepareGroup("web", time.Now())
	if err == nil {
		t.Fatal("ожидалась ошибка при отсутствии builds_folder у frontend")
	}
	if !containsRelease(prepared, "backend") {
		t.Fatalf("backend должен быть подготовлен до ошибки: %+v", prepared)
	}
	if containsRelease(prepared, "frontend") {
		t.Fatalf("frontend не должен быть подготовлен: %+v", prepared)
	}
}

// containsRelease проверяет наличие имени релиза в результатах подготовки.
func containsRelease(prepared []Prepared, name string) bool {
	for _, p := range prepared {
		if p.Release == name {
			return true
		}
	}
	return false
}
