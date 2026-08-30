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

// withEmptyPath подменяет PATH на пустую директорию, чтобы системные рантаймы
// (go, php) не находились. Используется в тестах, проверяющих выбор исключительно
// из скачанных в dev-command версий.
func withEmptyPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir)
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })
}

// TestPhpProgramNoNetwork проверяет, что фабрика PhpProgram не обращается
// к сети и заполняет известные поля.
func TestPhpProgramNoNetwork(t *testing.T) {
	p := PhpProgram("8.3")
	if p.Name() != "php" {
		t.Errorf("Name() = %q, want php", p.Name())
	}
	if p.Version() != "8.3" {
		t.Errorf("Version() = %q, want 8.3", p.Version())
	}
	if p.Binary() != "usr/bin/php8.3" {
		t.Errorf("Binary() = %q, want usr/bin/php8.3", p.Binary())
	}
	if p.URL() != "" || p.Archive() != "" {
		t.Errorf("URL/Archive должны быть пустыми до резолюции, got URL=%q Archive=%q", p.URL(), p.Archive())
	}
}

// TestGoProgramNoNetwork проверяет фабрику GoProgram без сети.
func TestGoProgramNoNetwork(t *testing.T) {
	p := GoProgram("1.22")
	if p.Name() != "go" {
		t.Errorf("Name() = %q, want go", p.Name())
	}
	if p.Version() != "1.22" {
		t.Errorf("Version() = %q, want 1.22", p.Version())
	}
	if p.Binary() != "go/bin/go" {
		t.Errorf("Binary() = %q, want go/bin/go", p.Binary())
	}
	if p.URL() != "" || p.Archive() != "" {
		t.Errorf("URL/Archive должны быть пустыми до резолюции, got URL=%q Archive=%q", p.URL(), p.Archive())
	}
}

// TestLnavProgram проверяет фабрику lnav: URL задан сразу (не требует сети).
func TestLnavProgram(t *testing.T) {
	p := LnavProgram()
	if p.Name() != "lnav" {
		t.Errorf("Name() = %q, want lnav", p.Name())
	}
	if p.Binary() != "lnav" {
		t.Errorf("Binary() = %q, want lnav", p.Binary())
	}
	if !strings.Contains(p.URL(), lnavVersion) {
		t.Errorf("URL() = %q, want contains %q", p.URL(), lnavVersion)
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

// TestBinaryPathAbsolute проверяет, что абсолютный путь к бинарю (системный
// рантайм) возвращается как есть, без добавления папки хранения.
func TestBinaryPathAbsolute(t *testing.T) {
	m := newTestManager(t)
	p := NewProgram("php", "8.3", "/usr/bin/php", "", "", "{php}")
	if got := m.BinaryPath(p); got != "/usr/bin/php" {
		t.Errorf("BinaryPath = %q, want /usr/bin/php", got)
	}
}

// TestCommandPlaceholders проверяет подстановку плейсхолдеров {имя}
// в полную команду программы.
func TestCommandPlaceholders(t *testing.T) {
	m := newTestManager(t)

	php := PhpProgram("8.3")
	phpstan := NewProgram("phpstan", "1.12.0", "phpstan.phar", "", "", "{php} {phpstan}", php)
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
	php := NewProgram("php", "", "", "", "", "")
	stan := NewProgram("phpstan", "", "", "", "", "", php)
	all := expandPrograms([]Executable{stan})
	if len(all) != 2 {
		t.Fatalf("expandPrograms length = %d, want 2", len(all))
	}
	if all[0].Name() != "phpstan" || all[1].Name() != "php" {
		t.Errorf("expandPrograms names = %v", []string{all[0].Name(), all[1].Name()})
	}
}

// TestRebuildWithDeps проверяет, что зависимости заменяются на актуальные версии.
func TestRebuildWithDeps(t *testing.T) {
	php := NewProgram("php", "8.1", "usr/bin/php8.1", "", "", "")
	stan := NewProgram("phpstan", "", "", "", "", "", php)
	flat := []Executable{stan, php}
	current := map[string]Executable{
		"phpstan": NewProgram("phpstan", "1.12.0", "phpstan.phar", "", "", ""),
		"php":     NewProgram("php", "8.2", "usr/bin/php8.2", "", "", ""),
	}
	result := rebuildWithDeps(flat, current)
	if len(result) != 2 {
		t.Fatalf("rebuildWithDeps length = %d, want 2", len(result))
	}
	req := result[0].Require()
	if len(req) != 1 {
		t.Fatalf("phpstan require = %v", req)
	}
	if got := req[0].Version(); got != "8.2" {
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
		{"1.22.2", "1.22", 0},
		{"1.22", "1.22.5", 0},
		{"1.23.1", "1.22", 1},
		{"1.21", "1.22.9", -1},
		{"8", "8.1", -1},
		{"9", "8.5", 1},
	}
	for _, c := range cases {
		if got := compareMajorMinor(c.a, c.b); got != c.want {
			t.Errorf("compareMajorMinor(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// installProgram создаёт в менеджере m установленную программу ex
// (создаёт папку и исполняемый файл по пути BinaryPath).
func installProgram(t *testing.T, m *Manager, ex Executable) {
	t.Helper()
	path := m.BinaryPath(ex)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
}

// TestReplaceDir проверяет атомарную замену папки версии: новое содержимое
// попадает на место старого, а временная папка исчезает.
func TestReplaceDir(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "go", "1.22")
	src := filepath.Join(root, ".install-123")

	// Старая (битая) версия уже существует на месте.
	if err := os.MkdirAll(filepath.Join(dst, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "bin", "go"), []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	// Готовим новую, полностью распакованную версию.
	if err := os.MkdirAll(filepath.Join(src, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "bin", "go"), []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := replaceDir(dst, src); err != nil {
		t.Fatalf("replaceDir: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dst, "bin", "go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Errorf("после replaceDir содержимое = %q, want new", data)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("временная папка должна быть перемещена, got err=%v", err)
	}
}

// TestRuntimeResolveMatchesMinor проверяет, что резолюция находит скачанную
// полную версию (1.22.12) по минорному требованию (1.22) без сети.
func TestRuntimeResolveMatchesMinor(t *testing.T) {
	withEmptyPath(t)
	m := newTestManager(t)
	installProgram(t, m, GoProgram("1.22.12"))

	rt := NewGo("1.22")
	resolved, ok := rt.Resolve(m.dir, "1.22")
	if !ok {
		t.Fatalf("Resolve(1.22) не нашла установленную 1.22.12")
	}
	if resolved.Version() != "1.22.12" {
		t.Errorf("resolved.Version = %q, want 1.22.12", resolved.Version())
	}
	if !m.IsInstalled(resolved) {
		t.Errorf("resolved программа должна быть установлена")
	}
}

// TestRuntimeResolveMismatch проверяет, что резолюция не находит версию,
// не удовлетворяющую требованию (1.21 не подходит под 1.22).
func TestRuntimeResolveMismatch(t *testing.T) {
	withEmptyPath(t)
	m := newTestManager(t)
	installProgram(t, m, GoProgram("1.21.4"))

	rt := NewGo("1.22")
	if _, ok := rt.Resolve(m.dir, "1.22"); ok {
		t.Fatalf("Resolve(1.22) не должна найти установленную 1.21.4")
	}
}

// TestRuntimeResolveFindsVersionFolder показывает, что наличие папки версии
// считается установленной версией (достаточно существования папки, файл не
// проверяется).
func TestRuntimeResolveFindsVersionFolder(t *testing.T) {
	withEmptyPath(t)
	m := newTestManager(t)
	// Создаём только папку версии, без бинаря.
	p := GoProgram("1.22.12")
	if err := os.MkdirAll(filepath.Join(m.dir, "go", p.Version()), 0755); err != nil {
		t.Fatal(err)
	}

	rt := NewGo("1.22")
	if _, ok := rt.Resolve(m.dir, "1.22"); !ok {
		t.Fatalf("Resolve(1.22) должна найти версию по наличию папки")
	}
}

// TestRuntimeResolveAnyForLatest проверяет, что при пустом требовании или
// "latest" подходит любая установленная версия.
func TestRuntimeResolveAnyForLatest(t *testing.T) {
	withEmptyPath(t)
	m := newTestManager(t)
	installProgram(t, m, GoProgram("1.24.2"))

	rt := NewGo("")
	for _, req := range []string{"", "latest"} {
		if _, ok := rt.Resolve(m.dir, req); !ok {
			t.Errorf("Resolve(%q) не нашла установленную версию", req)
		}
	}
}

// TestSatisfiesExact проверяет стратегию точного совпадения major.minor для Go.
func TestSatisfiesExact(t *testing.T) {
	cases := []struct {
		installed, required string
		want                bool
	}{
		{"1.22.12", "1.22", true},
		{"1.22", "1.22.5", true},
		{"1.23.1", "1.22", false},
		{"1.23", "1.22", false},
		{"2.0", "1.22", false},
		{"1.21.4", "", true},
		{"1.21.4", "latest", true},
	}
	for _, c := range cases {
		if got := NewGo("").Satisfies(c.installed, c.required); got != c.want {
			t.Errorf("Go.Satisfies(%q, %q) = %v, want %v", c.installed, c.required, got, c.want)
		}
	}
}

// TestSatisfiesAtLeast проверяет стратегию "минорно старшая или равная" для PHP.
func TestSatisfiesAtLeast(t *testing.T) {
	cases := []struct {
		installed, required string
		want                bool
	}{
		{"8.3", "8.2", true},
		{"8.3", "8.3", true},
		{"8.3", "8.5", false},
		{"8.2", "8.3", false},
		{"9.0", "8.3", false},
		{"8.3", "9.0", false},
		{"8.3", "", true},
		{"8.3", "latest", true},
	}
	for _, c := range cases {
		if got := NewPhp("").Satisfies(c.installed, c.required); got != c.want {
			t.Errorf("Php.Satisfies(%q, %q) = %v, want %v", c.installed, c.required, got, c.want)
		}
	}
}

// TestEnsureUsesSystemPhp показывает, что при наличии подходящего системного
// php Manager.Ensure использует его и НЕ скачивает рантайм в dev-команду.
func TestEnsureUsesSystemPhp(t *testing.T) {
	// Системный php 8.3 доступен в PATH (fake-бинарь).
	addFakeBinary(t, "php", "8.3")
	m := newTestManager(t)

	programs, err := m.Ensure(PhpProgram("8.3"))
	if err != nil {
		t.Fatalf("Ensure с системным php не должна обращаться к сети: %v", err)
	}
	if len(programs) != 1 {
		t.Fatalf("Ensure вернула %d программ, want 1", len(programs))
	}
	got := programs[0]
	if got.Name() != "php" {
		t.Errorf("Name() = %q, want php", got.Name())
	}
	if got.Version() != "8.3" {
		t.Errorf("Version() = %q, want 8.3", got.Version())
	}
	// Бинарь должен быть системным (абсолютный путь).
	binary := m.BinaryPath(got)
	if !filepath.IsAbs(binary) {
		t.Errorf("BinaryPath = %q, want абсолютный путь системного php", binary)
	}
}

// TestEnsurePrefersSystemOverDownloaded показывает, что системный рантайм идёт
// первым в списке кандидатов и выбирается раньше скачанного в dev-command.
func TestEnsurePrefersSystemOverDownloaded(t *testing.T) {
	// Системный php 8.3 доступен в PATH, и в dev-command тоже скачана 8.3.
	addFakeBinary(t, "php", "8.3")
	m := newTestManager(t)
	installProgram(t, m, PhpProgram("8.3"))

	rt := NewPhp("8.3")
	resolved, ok := rt.Resolve(m.dir, "8.3")
	if !ok {
		t.Fatalf("Resolve(8.3) не нашла ни одного кандидата")
	}
	sys, isRT := resolved.(Runtime)
	if !isRT || !sys.IsSystem() {
		t.Errorf("выбран не системный рантайм, а скачанный: %v", m.BinaryPath(resolved))
	}
}

// TestEnsureDoesNotDownloadUnmatchedSystemPhp показывает, что системный php,
// не удовлетворяющий требованию, не выбирается.
func TestEnsureDoesNotDownloadUnmatchedSystemPhp(t *testing.T) {
	// Системный php 8.2 младше требуемой 8.3.
	addFakeBinary(t, "php", "8.2")
	m := newTestManager(t)

	rt := NewPhp("8.3")
	if _, ok := rt.Resolve(m.dir, "8.3"); ok {
		t.Errorf("Resolve(php 8.2 под требование 8.3) = true, want false")
	}
}

// TestRuntimeResolveGoExact показывает, что для go требуется точная минорная
// версия: системный go 1.23 не подходит под требование 1.22.
func TestRuntimeResolveGoExact(t *testing.T) {
	// Системный go 1.23 доступен в PATH.
	addFakeBinary(t, "go", "go1.23")
	m := newTestManager(t)

	rt := NewGo("1.22")
	if _, ok := rt.Resolve(m.dir, "1.22"); ok {
		t.Errorf("Resolve(go 1.23 под требование 1.22) = true, want false (нужна точная версия)")
	}
}
