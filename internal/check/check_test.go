package check

import (
	"strings"
	"testing"
)

// TestMatchAvailableVersion проверяет сопоставление требуемой версии
// с ближайшей доступной.
func TestMatchAvailableVersion(t *testing.T) {
	available := []string{"8.5", "8.4", "8.3"}
	cases := []struct {
		required string
		want     string
	}{
		{"", "8.5"},    // пусто — берём новейшую
		{"8.1", "8.3"}, // ниже всех — минимальную доступную
		{"8.3", "8.3"},
		{"8.4", "8.4"},
		{"8.6", "8.5"}, // выше всех — новейшую
	}
	for _, c := range cases {
		got := matchAvailableVersion(c.required, available)
		if got != c.want {
			t.Errorf("matchAvailableVersion(%q) = %q, want %q", c.required, got, c.want)
		}
	}
}

// TestCompareMajorMinor проверяет сравнение версий вида major.minor.
func TestCompareMajorMinor(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"8.3", "8.4", -1},
		{"8.5", "8.3", 1},
		{"8.3", "8.3", 0},
	}
	for _, c := range cases {
		if got := compareMajorMinor(c.a, c.b); got != c.want {
			t.Errorf("compareMajorMinor(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestRunCommand_Placeholders проверяет подстановку плейсхолдеров {имя}
// в полную команду программы.
func TestRunCommand_Placeholders(t *testing.T) {
	dir := "/tmp/check"

	phpstan := Program{
		Name:        "phpstan",
		Binary:      "phpstan.phar",
		FullCommand: "{php} {phpstan}",
		Require:     []Program{{Name: "php", Binary: "php"}},
	}
	name, args := phpstan.runCommand(dir, nil)
	if name != "/tmp/check/php" {
		t.Errorf("runCommand phpstan name = %q, want %q", name, "/tmp/check/php")
	}
	if len(args) != 1 || args[0] != "/tmp/check/phpstan.phar" {
		t.Errorf("runCommand phpstan args = %v, want [%q]", args, "/tmp/check/phpstan.phar")
	}

	golangci := Program{
		Name:        "golangci-lint",
		Binary:      "golangci-lint",
		FullCommand: "{golangci-lint}",
	}
	name, args = golangci.runCommand(dir, []string{"run", "./..."})
	if name != "/tmp/check/golangci-lint" {
		t.Errorf("runCommand golangci-lint name = %q", name)
	}
	if len(args) != 2 || args[0] != "run" || args[1] != "./..." {
		t.Errorf("runCommand golangci-lint args = %v", args)
	}
}

// TestBuildArgs_Golangci проверяет построение аргументов golangci-lint.
func TestBuildArgs_Golangci(t *testing.T) {
	prog := goLinter()

	// Весь код в dry-run.
	args := buildArgs(prog, Scope{Name: "all"}, ModeDryRun)
	if got := strings.Join(args, " "); got != "run ./..." {
		t.Errorf("golangci dry-run all args = %q, want %q", got, "run ./...")
	}

	// Изменённые директории в dry-run
	scope := Scope{Name: "changed", Dirs: []string{"internal", "cmd"}}
	args = buildArgs(prog, scope, ModeDryRun)
	if got := strings.Join(args, " "); got != "run internal cmd" {
		t.Errorf("golangci dry-run dirs args = %q, want %q", got, "run internal cmd")
	}

	// Fix-режим.
	args = buildArgs(prog, scope, ModeFix)
	if got := strings.Join(args, " "); got != "run --fix internal cmd" {
		t.Errorf("golangci fix args = %q, want %q", got, "run --fix internal cmd")
	}
}

// TestBuildArgs_Phpstan проверяет построение аргументов PHPStan.
func TestBuildArgs_Phpstan(t *testing.T) {
	prog := phpStanLinter("")

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
	prog := phpCsFixerLinter("")

	// Dry-run добавляет флаг --dry-run.
	args := buildArgs(prog, Scope{Name: "all"}, ModeDryRun)
	if got := strings.Join(args, " "); got != "fix --dry-run ." {
		t.Errorf("php-cs-fixer dry-run all args = %q, want %q", got, "fix --dry-run .")
	}

	// Fix-режим убирает --dry-run.
	scope := Scope{Name: "changed", Files: []string{"src/a.php"}}
	args = buildArgs(prog, scope, ModeFix)
	if got := strings.Join(args, " "); got != "fix src/a.php" {
		t.Errorf("php-cs-fixer fix args = %q, want %q", got, "fix src/a.php")
	}
}

// TestProgramsFor проверяет набор линтеров для языков.
func TestProgramsFor(t *testing.T) {
	if progs := programsFor("go", ""); len(progs) != 1 || progs[0].Name != "golangci-lint" {
		t.Errorf("programsFor(go) = %v", progs)
	}
	// Для PHP возвращаются только линтеры; php лежит в их Require (вендорах).
	progs := programsFor("php", "8.3")
	if len(progs) != 2 {
		t.Errorf("programsFor(php) length = %d, want 2", len(progs))
	} else if progs[0].Name != "phpstan" || progs[1].Name != "php-cs-fixer" {
		t.Errorf("programsFor(php) names = %v", progs)
	}
	if progs := programsFor("ruby", ""); progs != nil {
		t.Errorf("programsFor(ruby) = %v, want nil", progs)
	}
}

// TestCollectPrograms проверяет сбор всех программ с вендорами (Require).
func TestCollectPrograms(t *testing.T) {
	php := Program{Name: "php", Binary: "php"}
	phpstan := Program{Name: "phpstan", Binary: "phpstan.phar", Require: []Program{php}}
	all := collectPrograms([]Program{phpstan})
	if len(all) != 2 {
		t.Fatalf("collectPrograms length = %d, want 2", len(all))
	}
	if all[0].Name != "phpstan" || all[1].Name != "php" {
		t.Errorf("collectPrograms names = %v", all)
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
