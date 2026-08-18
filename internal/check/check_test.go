package check

import (
	"strings"
	"testing"
)

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
	prog := phpStanLinter(Program{Name: "php"})

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
	prog := phpCsFixerLinter(Program{Name: "php"})

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
// Фабрика PhpProgram не обращается к сети, поэтому тест безопасен.
func TestProgramsFor(t *testing.T) {
	progs, err := programsFor("go", "")
	if err != nil || len(progs) != 1 || progs[0].Name != "golangci-lint" {
		t.Errorf("programsFor(go) = %v, err = %v", progs, err)
	}

	// Для PHP возвращаются только линтеры; php лежит в их Require (вендорах).
	progs, err = programsFor("php", "8.3")
	if err != nil {
		t.Fatalf("programsFor(php) error = %v", err)
	}
	if len(progs) != 2 {
		t.Errorf("programsFor(php) length = %d, want 2", len(progs))
	} else if progs[0].Name != "phpstan" || progs[1].Name != "php-cs-fixer" {
		t.Errorf("programsFor(php) names = %v", progs)
	}

	progs, err = programsFor("ruby", "")
	if err != nil || progs != nil {
		t.Errorf("programsFor(ruby) = %v, err = %v", progs, err)
	}
}

// TestPhpStanRequiresPhp проверяет, что PHPStan зависит от php-рантайма.
func TestPhpStanRequiresPhp(t *testing.T) {
	prog := phpStanLinter(Program{Name: "php", Version: "8.3"})
	if len(prog.Require) != 1 || prog.Require[0].Name != "php" {
		t.Errorf("phpstan require = %v, want php", prog.Require)
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
