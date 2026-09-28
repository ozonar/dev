package check

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeGoFile создаёт директорию и Go-файл в ней.
func writeGoFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// makeModule создаёт корень Go-модуля (go.mod) в указанной директории.
func makeModule(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", root, err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module m\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
}

// TestFindGoModuleRoot проверяет поиск корня Go-модуля: go.mod в самой
// директории, в родительской и отсутствие модуля.
func TestFindGoModuleRoot(t *testing.T) {
	tmp := t.TempDir()
	makeModule(t, filepath.Join(tmp, "server"))
	writeGoFile(t, filepath.Join(tmp, "server", "internal", "game", "a.go"))
	// Папка без go.mod (но с соседом-модулем).
	if err := os.MkdirAll(filepath.Join(tmp, "web"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	wantRoot := filepath.Join(tmp, "server")

	// Корень модуля — сама директория с go.mod.
	if got, ok := findGoModuleRoot(wantRoot); !ok || got != wantRoot {
		t.Errorf("findGoModuleRoot(%q) = %q, %v; want %q, true", wantRoot, got, ok, wantRoot)
	}

	// Подпапка модуля — корень находится подъёмом вверх.
	deep := filepath.Join(tmp, "server", "internal", "game")
	if got, ok := findGoModuleRoot(deep); !ok || got != wantRoot {
		t.Errorf("findGoModuleRoot(%q) = %q, %v; want %q, true", deep, got, ok, wantRoot)
	}

	// Директория вне модуля — go.mod не найден.
	if _, ok := findGoModuleRoot(filepath.Join(tmp, "web")); ok {
		t.Error("findGoModuleRoot(web) = true, want false (no go.mod above)")
	}

	// Несуществующая директория — модуля нет.
	if _, ok := findGoModuleRoot(filepath.Join(tmp, "missing")); ok {
		t.Error("findGoModuleRoot(missing) = true, want false")
	}
}

// TestGoLintRuns_All проверяет, что полный объём (scopeAll) даёт по одному
// запуску ./... на каждый найденный корень модуля, даже если корень модуля
// лежит в подпапке проекта (монорепо).
func TestGoLintRuns_All(t *testing.T) {
	tmp := t.TempDir()
	makeModule(t, filepath.Join(tmp, "server"))
	writeGoFile(t, filepath.Join(tmp, "server", "cmd", "main.go"))
	writeGoFile(t, filepath.Join(tmp, "server", "internal", "game", "a.go"))
	makeModule(t, filepath.Join(tmp, "tools"))
	writeGoFile(t, filepath.Join(tmp, "tools", "util", "x.go"))
	if err := os.MkdirAll(filepath.Join(tmp, "web"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	scope := Scope{Name: "all", kind: scopeAll, Dirs: []string{
		filepath.Join(tmp, "server"),
		filepath.Join(tmp, "server", "cmd"),
		filepath.Join(tmp, "server", "internal", "game"),
		filepath.Join(tmp, "tools"),
		filepath.Join(tmp, "tools", "util"),
		filepath.Join(tmp, "web"),
	}}

	runs := goLintRuns(scope)
	if len(runs) != 2 {
		t.Fatalf("goLintRuns(all) = %d runs, want 2: %+v", len(runs), runs)
	}

	wantServer := goLintRun{Dir: filepath.Join(tmp, "server"), Paths: []string{"./..."}}
	wantTools := goLintRun{Dir: filepath.Join(tmp, "tools"), Paths: []string{"./..."}}
	if !reflect.DeepEqual(runs[0], wantServer) || !reflect.DeepEqual(runs[1], wantTools) {
		t.Errorf("goLintRuns(all) = %+v, want [%+v, %+v]", runs, wantServer, wantTools)
	}
}

// TestGoLintRuns_Changed проверяет, что изменённые директории группируются
// по корню модуля, пути пересчитываются относительно него, а директории без
// Go-файлов и вне модуля отбрасываются.
func TestGoLintRuns_Changed(t *testing.T) {
	tmp := t.TempDir()
	makeModule(t, filepath.Join(tmp, "server"))
	writeGoFile(t, filepath.Join(tmp, "server", "cmd", "main.go"))
	writeGoFile(t, filepath.Join(tmp, "server", "internal", "game", "a.go"))
	// Каталог без .go-файлов (например изменён go.mod или README в нём).
	if err := os.MkdirAll(filepath.Join(tmp, "server", "docs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Каталог вне какого-либо модуля.
	if err := os.MkdirAll(filepath.Join(tmp, "web"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	scope := Scope{Name: "changed", Dirs: []string{
		filepath.Join(tmp, "server", "cmd"),
		filepath.Join(tmp, "server", "internal", "game"),
		filepath.Join(tmp, "server", "docs"),
		filepath.Join(tmp, "web"),
	}}

	runs := goLintRuns(scope)
	if len(runs) != 1 {
		t.Fatalf("goLintRuns(changed) = %d runs, want 1: %+v", len(runs), runs)
	}

	want := goLintRun{
		Dir:   filepath.Join(tmp, "server"),
		Paths: []string{"cmd", "internal/game"},
	}
	if !reflect.DeepEqual(runs[0], want) {
		t.Errorf("goLintRuns(changed) = %+v, want %+v", runs[0], want)
	}
}

// TestGoLintRuns_ChangedRelative проверяет группировку при относительных
// путях scope — именно такие пути приходят из git (относительно корня
// репозитория) при реальном запуске dev review. Относительный путь
// директории должен нормализоваться к абсолютному перед filepath.Rel,
// иначе корень модуля не найдётся.
func TestGoLintRuns_ChangedRelative(t *testing.T) {
	tmp := t.TempDir()
	makeModule(t, filepath.Join(tmp, "server"))
	writeGoFile(t, filepath.Join(tmp, "server", "cmd", "main.go"))
	writeGoFile(t, filepath.Join(tmp, "server", "internal", "game", "a.go"))

	// Переходим в корень проекта (как при запуске dev из него).
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	scope := Scope{Name: "changed", Dirs: []string{"server/cmd", "server/internal/game"}}
	runs := goLintRuns(scope)
	if len(runs) != 1 {
		t.Fatalf("goLintRuns(relative) = %d runs, want 1: %+v", len(runs), runs)
	}

	want := goLintRun{
		Dir:   filepath.Join(tmp, "server"),
		Paths: []string{"cmd", "internal/game"},
	}
	if !reflect.DeepEqual(runs[0], want) {
		t.Errorf("goLintRuns(relative) = %+v, want %+v", runs[0], want)
	}
}

// TestGoLintRuns_MultiModule проверяет, что изменения в нескольких модулях
// дают отдельный запуск на каждый модуль.
func TestGoLintRuns_MultiModule(t *testing.T) {
	tmp := t.TempDir()
	makeModule(t, filepath.Join(tmp, "server"))
	writeGoFile(t, filepath.Join(tmp, "server", "internal", "game", "a.go"))
	makeModule(t, filepath.Join(tmp, "tools"))
	writeGoFile(t, filepath.Join(tmp, "tools", "util", "x.go"))

	scope := Scope{Name: "changed", Dirs: []string{
		filepath.Join(tmp, "server", "internal", "game"),
		filepath.Join(tmp, "tools", "util"),
	}}

	runs := goLintRuns(scope)
	if len(runs) != 2 {
		t.Fatalf("goLintRuns(multi) = %d runs, want 2: %+v", len(runs), runs)
	}
	if runs[0].Dir != filepath.Join(tmp, "server") || runs[1].Dir != filepath.Join(tmp, "tools") {
		t.Errorf("goLintRuns(multi) dirs = %q, %q; want server, tools", runs[0].Dir, runs[1].Dir)
	}
	if !reflect.DeepEqual(runs[0].Paths, []string{"internal/game"}) {
		t.Errorf("server paths = %v, want [internal/game]", runs[0].Paths)
	}
	if !reflect.DeepEqual(runs[1].Paths, []string{"util"}) {
		t.Errorf("tools paths = %v, want [util]", runs[1].Paths)
	}
}

// TestGoLintRuns_NoModule проверяет, что при отсутствии go.mod (код вне
// модуля) запусков нет — golangci-lint пропускается, а не падает с ошибкой
// "no go files to analyze".
func TestGoLintRuns_NoModule(t *testing.T) {
	tmp := t.TempDir()
	writeGoFile(t, filepath.Join(tmp, "src", "a.go"))

	for _, kind := range []scopeKind{scopeAll, scopeChanged} {
		scope := Scope{Name: "no module", kind: kind, Dirs: []string{filepath.Join(tmp, "src")}}
		if runs := goLintRuns(scope); len(runs) != 0 {
			t.Errorf("goLintRuns(kind=%d) = %+v, want none (no go.mod)", kind, runs)
		}
	}
}

// TestGoLintRuns_NoGoDirs проверяет, что для изменённого кода без директорий
// с Go-файлами запусков нет.
func TestGoLintRuns_NoGoDirs(t *testing.T) {
	tmp := t.TempDir()
	makeModule(t, filepath.Join(tmp, "server"))

	// В scope только каталоги без .go-файлов (изменён go.mod или README).
	scope := Scope{Name: "changed", Dirs: []string{filepath.Join(tmp, "server")}}
	if runs := goLintRuns(scope); len(runs) != 0 {
		t.Errorf("goLintRuns(no go dirs) = %+v, want none", runs)
	}
}

// TestBuildGoLintArgs проверяет аргументы golangci-lint: dry-run без --fix,
// fix-режим с --fix, пути сохраняются в порядке передачи.
func TestBuildGoLintArgs(t *testing.T) {
	run := goLintRun{Dir: "/m", Paths: []string{"a", "b"}}

	if got := strings.Join(buildGoLintArgs(run, ModeDryRun), " "); got != "run a b" {
		t.Errorf("dry-run args = %q, want %q", got, "run a b")
	}
	if got := strings.Join(buildGoLintArgs(run, ModeFix), " "); got != "run --fix a b" {
		t.Errorf("fix args = %q, want %q", got, "run --fix a b")
	}
}
