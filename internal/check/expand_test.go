package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dev/internal/ai"
)

// writeTempFile создаёт файл с заданным содержимым и возвращает его
// абсолютный путь. Абсолютный путь позволяет строить diff в тестах
// без изменения рабочей директории процесса.
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// diffForFile формирует минимальный unified diff для одного файла.
func diffForFile(path, hunk string) string {
	return "diff --git a/" + path + " b/" + path + "\n" +
		"--- a/" + path + "\n" +
		"+++ b/" + path + "\n" +
		hunk
}

func TestParseChangedLines(t *testing.T) {
	diff := "diff --git a/foo.go b/foo.go\n" +
		"--- a/foo.go\n" +
		"+++ b/foo.go\n" +
		"@@ -1,3 +1,4 @@\n" +
		" package foo\n" +
		"+// changed\n" +
		" func a() {\n" +
		" }\n"

	got := parseChangedLines(diff)
	if len(got) != 1 {
		t.Fatalf("files = %d, want 1", len(got))
	}
	if got[0].path != "foo.go" {
		t.Errorf("path = %q, want foo.go", got[0].path)
	}
	want := []int{2}
	if len(got[0].lines) != len(want) || got[0].lines[0] != want[0] {
		t.Errorf("lines = %v, want %v", got[0].lines, want)
	}
}

func TestParseChangedLinesSkipsDeletedFile(t *testing.T) {
	diff := "diff --git a/gone.go b/gone.go\n" +
		"--- a/gone.go\n" +
		"+++ /dev/null\n" +
		"@@ -1 +0,0 @@\n" +
		"-package gone\n"

	if got := parseChangedLines(diff); len(got) != 0 {
		t.Errorf("files = %v, want none for deleted file", got)
	}
}

func TestFunctionBoundsGoBraces(t *testing.T) {
	lines := []string{
		"package foo",
		"",
		"func a() {",
		"\tx := 1",
		"}",
		"",
		"func b() {",
		"\ty := 2",
		"}",
	}

	start, end := functionBounds(lines, 3)
	if start != 2 || end != 4 {
		t.Errorf("bounds for line 3 = %d..%d, want 2..4", start, end)
	}

	start, end = functionBounds(lines, 7)
	if start != 6 || end != 8 {
		t.Errorf("bounds for line 7 = %d..%d, want 6..8", start, end)
	}
}

func TestFunctionBoundsPythonIndent(t *testing.T) {
	lines := []string{
		"def a():",
		"    x = 1",
		"",
		"def b():",
		"    y = 2",
	}

	start, end := functionBounds(lines, 1)
	if start != 0 {
		t.Errorf("start = %d, want 0", start)
	}
	if end < 1 || end > 2 {
		t.Errorf("end = %d, want 1..2", end)
	}
}

func TestExpandChangesToFunctionsAddsWholeFunction(t *testing.T) {
	path := writeTempFile(t, "sample.go", "package foo\n\nfunc a() {\n\tx := 2\n}\n\nfunc b() {\n\ty := 2\n}\n")
	base := diffForFile(path,
		"@@ -2,5 +2,5 @@\n"+
			" \n"+
			" func a() {\n"+
			"-	 x := 1\n"+
			"+	 x := 2\n"+
			" }\n")

	budget := len([]rune(base)) + 500
	got, added := expandChangesToFunctions(base, budget)
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	if !strings.Contains(got, "func a() {") || !strings.Contains(got, "x := 2") {
		t.Errorf("expanded text does not contain whole function a:\n%s", got)
	}
	if !strings.Contains(got, "function context") {
		t.Errorf("expanded text has no context header:\n%s", got)
	}
	if !strings.Contains(got, "3| func a() {") {
		t.Errorf("expanded text has no line numbers:\n%s", got)
	}
}

func TestExpandChangesToFunctionsRespectsBudget(t *testing.T) {
	path := writeTempFile(t, "sample.go", "package foo\n\nfunc a() {\n\tx := 2\n}\n")
	base := diffForFile(path,
		"@@ -2,3 +2,3 @@\n"+
			" \n"+
			"-	 x := 1\n"+
			"+	 x := 2\n")

	got, added := expandChangesToFunctions(base, len([]rune(base)))
	if added != 0 {
		t.Errorf("added = %d, want 0 when budget equals base size", added)
	}
	if got != base {
		t.Errorf("text changed despite zero budget")
	}
}

func TestExpandChangesToFunctionsMergesSameFunction(t *testing.T) {
	path := writeTempFile(t, "sample.go", "package foo\n\nfunc a() {\n\tx := 2\n\ty := 3\n\tz := 4\n}\n")
	base := diffForFile(path,
		"@@ -3,5 +3,5 @@\n"+
			" func a() {\n"+
			"-\tx := 1\n"+
			"+\tx := 2\n"+
			" \ty := 3\n"+
			"-\tz := 3\n"+
			"+\tz := 4\n"+
			" }\n")

	got, added := expandChangesToFunctions(base, len([]rune(base))+1000)
	if added != 1 {
		t.Fatalf("added = %d, want 1 (two changes in one function)", added)
	}
	if strings.Count(got, "function context") != 1 {
		t.Errorf("expected exactly one function block:\n%s", got)
	}
}

func TestExpandChangesToFunctionsNonDiff(t *testing.T) {
	const text = "just some code without a diff"
	got, added := expandChangesToFunctions(text, 1000)
	if added != 0 {
		t.Errorf("added = %d, want 0 for non-diff text", added)
	}
	if got != text {
		t.Errorf("text = %q, want unchanged", got)
	}
}

// TestScopeSizeHintForAI_ExpandsToFunctions проверяет, что подсказка меню
// при включённом расширении показывает размер текста вместе с добавленными
// функциями, а без расширения — размер базового diff.
func TestScopeSizeHintForAI_ExpandsToFunctions(t *testing.T) {
	tmp := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		if err := gitRun(tmp, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	original := "package main\n\nfunc main() {\n\tx := 1\n\t_ = x\n}\n"
	writeFile(t, filepath.Join(tmp, "main.go"), original)
	if err := gitRun(tmp, "add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := gitRun(tmp, "commit", "-qm", "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	writeFile(t, filepath.Join(tmp, "main.go"), strings.Replace(original, "x := 1", "x := 2", 1))

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	opt := scopeOption{kind: scopeChanged}
	base := scopeSizeHintForAI(false, ai.MaxReviewTotalChars)(opt)
	expanded := scopeSizeHintForAI(true, ai.MaxReviewTotalChars)(opt)

	if base == "" {
		t.Fatal("base hint is empty")
	}
	if expanded == base {
		t.Errorf("expanded hint = %q, want a larger size than base %q", expanded, base)
	}
}

// TestChangedDiffTextIncludesUntracked проверяет, что новые файлы попадают
// в текст ревью, даже если они не добавлены в git.
func TestChangedDiffTextIncludesUntracked(t *testing.T) {
	tmp := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		if err := gitRun(tmp, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	writeFile(t, filepath.Join(tmp, "main.go"), "package main\n")
	if err := gitRun(tmp, "add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := gitRun(tmp, "commit", "-qm", "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	writeFile(t, filepath.Join(tmp, "new.go"), "package main\n\nfunc New() {}\n")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	text := changedDiffText([]string{"new.go"})
	if !strings.Contains(text, "func New() {}") {
		t.Errorf("changedDiffText does not include untracked file content:\n%s", text)
	}
	if !strings.Contains(text, "new file") {
		t.Errorf("changedDiffText has no new-file marker:\n%s", text)
	}
}
