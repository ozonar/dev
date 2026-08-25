package check

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dev/internal/toolchain"
)

// TestBuildArgs_Golangci проверяет построение аргументов golangci-lint.
func TestBuildArgs_Golangci(t *testing.T) {
	prog := goLinter(toolchain.NewGo(""))

	// Весь код в dry-run.
	args := buildArgs(prog, Scope{Name: "all"}, ModeDryRun)
	if got := strings.Join(args, " "); got != "run ./..." {
		t.Errorf("golangci dry-run all args = %q, want %q", got, "run ./...")
	}

	// Директория с Go-файлом проходит фильтр, директория без — отбрасывается.
	withGo := filepath.Join(t.TempDir(), "pkg")
	if err := os.MkdirAll(withGo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(withGo, "a.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	emptyDir := filepath.Join(t.TempDir(), "nogo")

	scope := Scope{Name: "changed", Dirs: []string{emptyDir, withGo}}
	args = buildArgs(prog, scope, ModeDryRun)
	if got, want := strings.Join(args, " "), "run "+withGo; got != want {
		t.Errorf("golangci dry-run dirs args = %q, want %q", got, want)
	}

	// Fix-режим.
	args = buildArgs(prog, scope, ModeFix)
	if got, want := strings.Join(args, " "), "run --fix "+withGo; got != want {
		t.Errorf("golangci fix args = %q, want %q", got, want)
	}

	// Если после фильтрации директорий не осталось — fallback на ./....
	scopeEmpty := Scope{Name: "changed", Dirs: []string{emptyDir}}
	args = buildArgs(prog, scopeEmpty, ModeDryRun)
	if got := strings.Join(args, " "); got != "run ./..." {
		t.Errorf("golangci empty dirs args = %q, want %q", got, "run ./...")
	}
}

// TestBuildArgs_Phpstan проверяет построение аргументов PHPStan.
func TestBuildArgs_Phpstan(t *testing.T) {
	prog := phpStanLinter(toolchain.NewPhp(""))

	args := buildArgs(prog, Scope{Name: "all"}, ModeDryRun)
	if got := strings.Join(args, " "); got != "analyse --memory-limit=1G --level=5 ." {
		t.Errorf("phpstan dry-run all args = %q, want %q", got, "analyse --memory-limit=1G --level=5 .")
	}

	scope := Scope{Name: "changed", Files: []string{"src/a.php"}}
	args = buildArgs(prog, scope, ModeFix)
	if got := strings.Join(args, " "); got != "analyse --memory-limit=1G --level=5 src/a.php" {
		t.Errorf("phpstan fix args = %q, want %q", got, "analyse --memory-limit=1G --level=5 src/a.php")
	}
}

// TestBuildArgs_PhpCsFixer проверяет построение аргументов PHP CS Fixer.
func TestBuildArgs_PhpCsFixer(t *testing.T) {
	prog := phpCsFixerLinter(toolchain.NewPhp(""))
	// Конфиг генерируется в папке девконфига и всегда передаётся через --config.
	cfg := ensurePhpCsFixerConfig(".")
	configArg := "--config=" + cfg

	// Dry-run добавляет флаг --dry-run и конфиг.
	args := buildArgs(prog, Scope{Name: "all"}, ModeDryRun)
	want := "fix --dry-run " + configArg + " ."
	if got := strings.Join(args, " "); got != want {
		t.Errorf("php-cs-fixer dry-run all args = %q, want %q", got, want)
	}

	// Fix-режим убирает --dry-run, но сохраняет конфиг и пути файлов.
	scope := Scope{Name: "changed", Files: []string{"src/a.php"}}
	args = buildArgs(prog, scope, ModeFix)
	want = "fix " + configArg + " src/a.php"
	if got := strings.Join(args, " "); got != want {
		t.Errorf("php-cs-fixer fix args = %q, want %q", got, want)
	}
}

// TestProgramsFor проверяет набор линтеров для языков.
// Фабрика PhpProgram не обращается к сети, поэтому тест безопасен.
func TestProgramsFor(t *testing.T) {
	progs, err := programsFor("go", "")
	if err != nil || len(progs) != 1 || progs[0].Name() != "golangci-lint" {
		t.Errorf("programsFor(go) = %v, err = %v", progs, err)
	}

	// Для PHP возвращаются только линтеры; php лежит в их Require (вендорах).
	progs, err = programsFor("php", "8.3")
	if err != nil {
		t.Fatalf("programsFor(php) error = %v", err)
	}
	if len(progs) != 2 {
		t.Errorf("programsFor(php) length = %d, want 2", len(progs))
	} else if progs[0].Name() != "phpstan" || progs[1].Name() != "php-cs-fixer" {
		t.Errorf("programsFor(php) names = %v", progs)
	}

	// JavaScript/TypeScript (Node) — один анализатор Biome.
	progs, err = programsFor("javascript", "")
	if err != nil {
		t.Fatalf("programsFor(javascript) error = %v", err)
	}
	if len(progs) != 1 || progs[0].Name() != "biome" {
		t.Errorf("programsFor(javascript) = %v, want [biome]", progs)
	}

	// Python — один анализатор Ruff.
	progs, err = programsFor("python", "")
	if err != nil {
		t.Fatalf("programsFor(python) error = %v", err)
	}
	if len(progs) != 1 || progs[0].Name() != "ruff" {
		t.Errorf("programsFor(python) = %v, want [ruff]", progs)
	}

	progs, err = programsFor("ruby", "")
	if err != nil || progs != nil {
		t.Errorf("programsFor(ruby) = %v, err = %v", progs, err)
	}
}

// TestBiomeLinter проверяет описание Biome: одиночный файл (без архива),
// исполняемый файл и корректный URL.
func TestBiomeLinter(t *testing.T) {
	prog := biomeLinter()
	if prog.Name() != "biome" {
		t.Errorf("biome name = %q", prog.Name())
	}
	if prog.Archive() != "" {
		t.Errorf("biome archive = %q, want empty (single file)", prog.Archive())
	}
	if prog.Binary() == "" || prog.URL() == "" {
		t.Errorf("biome binary/url empty: %+v", prog)
	}
	if prog.FullCommand() != "{biome}" {
		t.Errorf("biome fullCommand = %q", prog.FullCommand())
	}
	if len(prog.Require()) != 0 {
		t.Errorf("biome require = %v, want none", prog.Require())
	}
}

// TestRuffLinter проверяет описание Ruff: tar.gz архив и корректный URL.
func TestRuffLinter(t *testing.T) {
	prog := ruffLinter()
	if prog.Name() != "ruff" {
		t.Errorf("ruff name = %q", prog.Name())
	}
	if prog.Archive() != "tar.gz" {
		t.Errorf("ruff archive = %q, want tar.gz", prog.Archive())
	}
	if prog.Binary() == "" || prog.URL() == "" {
		t.Errorf("ruff binary/url empty: %+v", prog)
	}
	if prog.FullCommand() != "{ruff}" {
		t.Errorf("ruff fullCommand = %q", prog.FullCommand())
	}
	if len(prog.Require()) != 0 {
		t.Errorf("ruff require = %v, want none", prog.Require())
	}
}

// TestBuildArgs_Biome проверяет построение аргументов Biome.
func TestBuildArgs_Biome(t *testing.T) {
	prog := biomeLinter()

	// Dry-run всех файлов: biome check .
	args := buildArgs(prog, Scope{Name: "all"}, ModeDryRun)
	if got := strings.Join(args, " "); got != "check ." {
		t.Errorf("biome dry-run all args = %q, want %q", got, "check .")
	}

	// Fix-режим добавляет --write.
	args = buildArgs(prog, Scope{Name: "all"}, ModeFix)
	if got := strings.Join(args, " "); got != "check --write ." {
		t.Errorf("biome fix all args = %q, want %q", got, "check --write .")
	}

	// Dry-run с явными файлами.
	scope := Scope{Name: "changed", Files: []string{"src/a.ts", "src/b.js"}}
	args = buildArgs(prog, scope, ModeDryRun)
	if got := strings.Join(args, " "); got != "check src/a.ts src/b.js" {
		t.Errorf("biome dry-run files args = %q, want %q", got, "check src/a.ts src/b.js")
	}
}

// TestBuildArgs_Ruff проверяет построение аргументов Ruff.
func TestBuildArgs_Ruff(t *testing.T) {
	prog := ruffLinter()

	// Dry-run всех файлов: ruff check .
	args := buildArgs(prog, Scope{Name: "all"}, ModeDryRun)
	if got := strings.Join(args, " "); got != "check ." {
		t.Errorf("ruff dry-run all args = %q, want %q", got, "check .")
	}

	// Fix-режим добавляет --fix.
	args = buildArgs(prog, Scope{Name: "all"}, ModeFix)
	if got := strings.Join(args, " "); got != "check --fix ." {
		t.Errorf("ruff fix all args = %q, want %q", got, "check --fix .")
	}

	// Dry-run с явными файлами.
	scope := Scope{Name: "changed", Files: []string{"src/a.py", "src/b.py"}}
	args = buildArgs(prog, scope, ModeDryRun)
	if got := strings.Join(args, " "); got != "check src/a.py src/b.py" {
		t.Errorf("ruff dry-run files args = %q, want %q", got, "check src/a.py src/b.py")
	}
}

// TestPhpStanRequiresPhp проверяет, что PHPStan зависит от php-рантайма.
func TestPhpStanRequiresPhp(t *testing.T) {
	prog := phpStanLinter(toolchain.NewPhp("8.3"))
	req := prog.Require()
	if len(req) != 1 || req[0].Name() != "php" {
		t.Errorf("phpstan require = %v, want php", req)
	}
}

// TestScopeConstructors проверяет конструкторы объёмов проверки.
func TestScopeConstructors(t *testing.T) {
	if s := ScopeAll(); s.Name != "all code" {
		t.Errorf("ScopeAll name = %q", s.Name)
	}
	if s := ScopeCommits(2); !strings.Contains(s.Name, "2 commits") {
		t.Errorf("ScopeCommits(2) name = %q", s.Name)
	}
	if s := ScopeDiff("master"); !strings.Contains(s.Name, "master") {
		t.Errorf("ScopeDiff(master) name = %q", s.Name)
	}
	if s := ScopeDiff("develop"); !strings.Contains(s.Name, "develop") {
		t.Errorf("ScopeDiff(develop) name = %q", s.Name)
	}
}

// TestGetChanges проверяет, что GetChanges возвращает нужный текст:
// для изменённых файлов — полный код файлов, для остальных — diff.
func TestGetChanges(t *testing.T) {
	// Scope "changed files" (kind == scopeFiles) возвращает полный код файлов.
	files := Scope{
		Name:        "changed files",
		Changes:     "diff-text",
		FileChanges: "full-file-code",
		kind:        scopeFiles,
	}
	if got := files.GetChanges(); got != "full-file-code" {
		t.Errorf("GetChanges for scopeFiles = %q, want %q", got, "full-file-code")
	}

	// Обычный scope возвращает diff (текст изменений).
	changed := Scope{
		Name:    "changed code",
		Changes: "diff-text",
		kind:    scopeChanged,
	}
	if got := changed.GetChanges(); got != "diff-text" {
		t.Errorf("GetChanges for scopeChanged = %q, want %q", got, "diff-text")
	}
}

// TestGoDirArgs проверяет, что goDirArgs отбрасывает директории без Go-файлов
// (включая корневую "." и несуществующие), оставляя только Go-пакеты.
func TestGoDirArgs(t *testing.T) {
	// Директория с .go-файлом проходит фильтр.
	withGo := filepath.Join(t.TempDir(), "pkg")
	if err := os.MkdirAll(withGo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(withGo, "a.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Директория только с не-Go файлом — отбрасывается.
	noGo := filepath.Join(t.TempDir(), "docs")
	if err := os.MkdirAll(noGo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(noGo, "readme.md"), []byte("# docs\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")

	got := goDirArgs([]string{noGo, withGo, missing})
	if len(got) != 1 || got[0] != withGo {
		t.Errorf("goDirArgs = %v, want [%s]", got, withGo)
	}

	// Пустой список возвращает пустой результат (buildArgs подставит ./...).
	if got := goDirArgs(nil); len(got) != 0 {
		t.Errorf("goDirArgs(nil) = %v, want empty", got)
	}

	// Только несуществующие/пустые директории — тоже пусто.
	if got := goDirArgs([]string{missing, noGo}); len(got) != 0 {
		t.Errorf("goDirArgs(no go dirs) = %v, want empty", got)
	}
}

// TestIsVendorPath проверяет распознавание путей внутри каталогов вендоров.
func TestIsVendorPath(t *testing.T) {
	vendor := []string{
		"vendor/github.com/pkg/errors/errors.go",
		"vendor/foo/bar.go",
		"src/vendor/a.go",
		"node_modules/lodash/index.js",
		"app/node_modules/pkg/main.ts",
		"venv/lib/python/site.py",
		".venv/bin/pip",
		"Pods/AFNetworking/AFNetworking.h",
		"bower_components/jquery/dist/jquery.js",
		"third_party/protobuf/foo.pb.go",
		"lib/site-packages/requests/api.py",
		"src/__pycache__/mod.cpython-311.pyc",
	}
	for _, p := range vendor {
		if !isVendorPath(p) {
			t.Errorf("isVendorPath(%q) = false, want true", p)
		}
	}

	notVendor := []string{
		"src/main.go",
		"cmd/dev/main.go",
		"README.md",
		"internal/pkg/core.go",
		"dist-not-vendor/file.txt", // имя начинается с "dist", но это не каталог dist
	}
	for _, p := range notVendor {
		if isVendorPath(p) {
			t.Errorf("isVendorPath(%q) = true, want false", p)
		}
	}
}

// TestFilterVendor проверяет, что filterVendor отбрасывает файлы из папок
// вендоров, сохраняя остальные.
func TestFilterVendor(t *testing.T) {
	input := []string{
		"src/main.go",
		"vendor/github.com/pkg/errors/errors.go",
		"cmd/dev/main.go",
		"node_modules/lodash/index.js",
		"internal/pkg/core.go",
		"venv/bin/activate",
	}
	got := filterVendor(input)
	want := []string{
		"src/main.go",
		"cmd/dev/main.go",
		"internal/pkg/core.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterVendor = %v, want %v", got, want)
	}
}

// TestDiffPathArgs проверяет построение аргументов diff для списка путей:
// добавляется разделитель "--" перед путями; пустой список даёт nil.
func TestDiffPathArgs(t *testing.T) {
	if got := diffPathArgs(nil); got != nil {
		t.Errorf("diffPathArgs(nil) = %v, want nil", got)
	}
	if got := diffPathArgs([]string{}); got != nil {
		t.Errorf("diffPathArgs(empty) = %v, want nil", got)
	}

	got := diffPathArgs([]string{"src/a.go", "cmd/main.go"})
	want := []string{"--", "src/a.go", "cmd/main.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("diffPathArgs = %v, want %v", got, want)
	}
}

// gitRun запускает git-команду в заданной директории и возвращает ошибку.
func gitRun(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}

// writeFile создаёт файл (и каталоги) и записывает содержимое.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestBuildScope_FiltersVendorDiff проверяет, что изменённый файл внутри
// папки вендора исключается из scope как из списка файлов, так и из diff.
func TestBuildScope_FiltersVendorDiff(t *testing.T) {
	tmp := t.TempDir()

	// Инициализируем git-репозиторий.
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		if err := gitRun(tmp, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	// Создаём и коммитим два файла: обычный и внутри vendor.
	writeFile(t, filepath.Join(tmp, "main.go"), "package main\nfunc main() {}\n")
	writeFile(t, filepath.Join(tmp, "vendor", "dep", "dep.go"), "package dep\n")
	if err := gitRun(tmp, "add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := gitRun(tmp, "commit", "-qm", "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// Вносим изменения в оба файла.
	writeFile(t, filepath.Join(tmp, "main.go"), "package main\n\nfunc main() { println(\"hi\") }\n")
	writeFile(t, filepath.Join(tmp, "vendor", "dep", "dep.go"), "package dep\n\n// vendor change\n")

	// Переключаем рабочую директорию на временный репозиторий и обязательно
	// восстанавливаем исходную после теста.
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	scope := buildScope(scopeChanged)

	// Файл вендора не должен попасть в список файлов.
	for _, f := range scope.Files {
		if isVendorPath(f) {
			t.Errorf("scope.Files содержит вендорный файл: %q", f)
		}
	}
	// Текст изменений не должен содержать упоминание вендорного файла.
	if strings.Contains(scope.Changes, "vendor/dep/dep.go") {
		t.Errorf("scope.Changes не должен содержать diff вендорного файла:\n%s", scope.Changes)
	}
	// Обычный файл должен остаться в списке.
	if len(scope.Files) == 0 {
		t.Fatal("scope.Files пуст, ожидался как минимум main.go")
	}
}
