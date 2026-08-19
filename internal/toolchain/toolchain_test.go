package toolchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestManager создаёт менеджер с временной папкой хранения.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{dir: t.TempDir()}
}

// TestPhpProgramNoNetwork проверяет, что фабрика PhpProgram не обращается
// к сети и заполняет известные поля.
func TestPhpProgramNoNetwork(t *testing.T) {
	p := PhpProgram("8.3")
	if p.Name != "php" {
		t.Errorf("Name = %q, want php", p.Name)
	}
	if p.Version != "8.3" {
		t.Errorf("Version = %q, want 8.3", p.Version)
	}
	if p.Binary != "usr/bin/php8.3" {
		t.Errorf("Binary = %q, want usr/bin/php8.3", p.Binary)
	}
	if p.URL != "" || p.Archive != "" {
		t.Errorf("URL/Archive должны быть пустыми до резолюции, got URL=%q Archive=%q", p.URL, p.Archive)
	}
}

// TestGoProgramNoNetwork проверяет фабрику GoProgram без сети.
func TestGoProgramNoNetwork(t *testing.T) {
	p := GoProgram("1.22")
	if p.Name != "go" {
		t.Errorf("Name = %q, want go", p.Name)
	}
	if p.Version != "1.22" {
		t.Errorf("Version = %q, want 1.22", p.Version)
	}
	if p.Binary != "go/bin/go" {
		t.Errorf("Binary = %q, want go/bin/go", p.Binary)
	}
	if p.URL != "" || p.Archive != "" {
		t.Errorf("URL/Archive должны быть пустыми до резолюции, got URL=%q Archive=%q", p.URL, p.Archive)
	}
}

// TestLnavProgram проверяет фабрику lnav: URL задан сразу (не требует сети).
func TestLnavProgram(t *testing.T) {
	p := LnavProgram()
	if p.Name != "lnav" {
		t.Errorf("Name = %q, want lnav", p.Name)
	}
	if p.Binary != "lnav" {
		t.Errorf("Binary = %q, want lnav", p.Binary)
	}
	if !strings.Contains(p.URL, lnavVersion) {
		t.Errorf("URL = %q, want contains %q", p.URL, lnavVersion)
	}
}

// TestProgramDir проверяет формирование пути хранения программы с учётом версии.
func TestProgramDir(t *testing.T) {
	m := newTestManager(t)
	p := PhpProgram("8.3")
	want := filepath.Join(m.dir, "php", "8.3")
	if got := m.programDir(p); got != want {
		t.Errorf("programDir = %q, want %q", got, want)
	}
}

// TestBinaryPath проверяет путь к исполняемому файлу программы.
func TestBinaryPath(t *testing.T) {
	m := newTestManager(t)
	p := PhpProgram("8.3")
	want := filepath.Join(m.dir, "php", "8.3", "usr/bin/php8.3")
	if got := m.BinaryPath(p); got != want {
		t.Errorf("BinaryPath = %q, want %q", got, want)
	}
}

// TestCommandPlaceholders проверяет подстановку плейсхолдеров {имя}
// в полную команду программы.
func TestCommandPlaceholders(t *testing.T) {
	m := newTestManager(t)

	php := Program{Name: "php", Version: "8.3", Binary: "usr/bin/php8.3"}
	phpstan := Program{
		Name:        "phpstan",
		Version:     "1.12.0",
		Binary:      "phpstan.phar",
		FullCommand: "{php} {phpstan}",
		Require:     []Program{php},
	}
	name, args := m.Command(phpstan, []string{"analyse", "."})
	wantName := filepath.Join(m.dir, "phpstan", "1.12.0", "phpstan.phar")
	wantPhp := filepath.Join(m.dir, "php", "8.3", "usr/bin/php8.3")
	if name != wantPhp {
		t.Errorf("command name = %q, want %q", name, wantPhp)
	}
	if len(args) != 3 || args[0] != wantName || args[1] != "analyse" || args[2] != "." {
		t.Errorf("command args = %v", args)
	}
}

// TestExpandPrograms проверяет разворачивание программ с зависимостями.
func TestExpandPrograms(t *testing.T) {
	php := Program{Name: "php"}
	stan := Program{Name: "phpstan", Require: []Program{php}}
	all := expandPrograms([]Program{stan})
	if len(all) != 2 {
		t.Fatalf("expandPrograms length = %d, want 2", len(all))
	}
	if all[0].Name != "phpstan" || all[1].Name != "php" {
		t.Errorf("expandPrograms names = %v", all)
	}
}

// TestRebuildWithDeps проверяет, что зависимости заменяются на актуальные версии.
func TestRebuildWithDeps(t *testing.T) {
	php := Program{Name: "php", Version: "8.1", Binary: "usr/bin/php8.1"}
	stan := Program{Name: "phpstan", Require: []Program{php}}
	flat := []Program{stan, php}
	current := map[string]Program{
		"phpstan": {Name: "phpstan", Version: "1.12.0"},
		"php":     {Name: "php", Version: "8.2", Binary: "usr/bin/php8.2"},
	}
	result := rebuildWithDeps(flat, current)
	if len(result) != 2 {
		t.Fatalf("rebuildWithDeps length = %d, want 2", len(result))
	}
	if result[0].Require == nil || len(result[0].Require) != 1 {
		t.Fatalf("phpstan require = %v", result[0].Require)
	}
	if got := result[0].Require[0].Version; got != "8.2" {
		t.Errorf("php dependency version = %q, want 8.2", got)
	}
}

// TestIsInstalled проверяет определение наличия установленной программы.
func TestIsInstalled(t *testing.T) {
	m := newTestManager(t)
	p := PhpProgram("8.3")
	if m.IsInstalled(p) {
		t.Error("IsInstalled = true для ещё не созданного файла")
	}
	// Создаём файл по ожидаемому пути.
	path := m.BinaryPath(p)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	if !m.IsInstalled(p) {
		t.Error("IsInstalled = false после создания файла")
	}
}

// TestCompareMajorMinor проверяет сравнение версий major.minor.
func TestCompareMajorMinor(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"8.3", "8.4", -1},
		{"8.5", "8.3", 1},
		{"8.3", "8.3", 0},
		// Патч-версия (три компонента) не должна влиять на сравнение major.minor:
		// системный go 1.22.2 подходит под требование go 1.22.
		{"1.22.2", "1.22", 0},
		{"1.22", "1.22.5", 0},
		{"1.23.1", "1.22", 1},
		{"1.21", "1.22.9", -1},
		// Версия без minor трактуется как major.0.
		{"8", "8.1", -1},
		{"9", "8.5", 1},
	}
	for _, c := range cases {
		if got := compareMajorMinor(c.a, c.b); got != c.want {
			t.Errorf("compareMajorMinor(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// installProgram создаёт в менеджере m установленную программу p
// (создаёт папку и исполняемый файл по пути BinaryPath).
func installProgram(t *testing.T, m *Manager, p Program) {
	t.Helper()
	path := m.BinaryPath(p)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
}

// TestResolveLocalMatchesMinor проверяет, что локальная резолюция находит
// скачанную полную версию (1.22.12) по минорному требованию (1.22) без сети.
func TestResolveLocalMatchesMinor(t *testing.T) {
	m := newTestManager(t)
	installProgram(t, m, GoProgram("1.22.12"))

	resolved, ok := m.resolveLocal(GoProgram("1.22"))
	if !ok {
		t.Fatalf("resolveLocal(1.22) не нашла установленную 1.22.12")
	}
	if resolved.Version != "1.22.12" {
		t.Errorf("resolved.Version = %q, want 1.22.12", resolved.Version)
	}
	if !m.IsInstalled(resolved) {
		t.Errorf("resolved программа должна быть установлена")
	}
}

// TestResolveLocalMismatch проверяет, что локальная резолюция не находит
// версию, не удовлетворяющую требованию (1.21 не подходит под 1.22).
func TestResolveLocalMismatch(t *testing.T) {
	m := newTestManager(t)
	installProgram(t, m, GoProgram("1.21.4"))

	if _, ok := m.resolveLocal(GoProgram("1.22")); ok {
		t.Fatalf("resolveLocal(1.22) не должна найти установленную 1.21.4")
	}
}

// TestResolveLocalIgnoresMissingFile проверяет, что папка версии без
// исполняемого файла игнорируется локальной резолюцией.
func TestResolveLocalIgnoresMissingFile(t *testing.T) {
	m := newTestManager(t)
	// Создаём только папку версии, без бинаря.
	p := GoProgram("1.22.12")
	if err := os.MkdirAll(filepath.Join(m.dir, "go", p.Version), 0755); err != nil {
		t.Fatal(err)
	}

	if _, ok := m.resolveLocal(GoProgram("1.22")); ok {
		t.Fatalf("resolveLocal(1.22) не должна найти версию без исполняемого файла")
	}
}

// TestResolveLocalAnyForLatest проверяет, что при пустом требовании или
// "latest" подходит любая установленная версия.
func TestResolveLocalAnyForLatest(t *testing.T) {
	m := newTestManager(t)
	installProgram(t, m, GoProgram("1.24.2"))

	for _, req := range []string{"", "latest"} {
		if _, ok := m.resolveLocal(GoProgram(req)); !ok {
			t.Errorf("resolveLocal(%q) не нашла установленную версию", req)
		}
	}
}

// TestMatchesRequirement проверяет сопоставление установленной и требуемой
// версии, включая случай полная (1.22.12) против минорной (1.22).
func TestMatchesRequirement(t *testing.T) {
	cases := []struct {
		installed, required string
		want                bool
	}{
		// Полная установленная версия удовлетворяет минорному требованию.
		{"1.22.12", "1.22", true},
		{"1.22", "1.22.5", true},
		// Разные major/minor — не подходит.
		{"1.21.4", "1.22", false},
		{"1.23.1", "1.22", false},
		// Пустое требование и "latest" — подходит любая установленная.
		{"1.21.4", "", true},
		{"1.21.4", "latest", true},
	}
	for _, c := range cases {
		if got := matchesRequirement(c.installed, c.required); got != c.want {
			t.Errorf("matchesRequirement(%q, %q) = %v, want %v", c.installed, c.required, got, c.want)
		}
	}
}
