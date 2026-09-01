//go:build !windows

package prepare

import (
	"os"
	"path/filepath"
	"testing"
)

// writeEnvFile создаёт файл в временной директории и возвращает его путь.
func writeEnvFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
	return path
}

// TestParseEnvVariables проверяет разбор переменных из .env файла:
// комментарии и пустые строки игнорируются, кавычки снимаются,
// префикс export не мешает, строки без '=' пропускаются.
func TestParseEnvVariables(t *testing.T) {
	dir := t.TempDir()
	p := writeEnvFile(t, dir, "source.env", `# comment
APP_ENV=prod
DB_HOST="localhost"
EMPTY=
export EXPORTED=1
not-a-var
`)
	vars, err := parseEnvVariables(p)
	if err != nil {
		t.Fatalf("parseEnvVariables returned error: %v", err)
	}
	want := map[string]string{
		"APP_ENV":  "prod",
		"DB_HOST":  "localhost",
		"EMPTY":    "",
		"EXPORTED": "1",
	}
	for k, v := range want {
		if got, ok := vars[k]; !ok || got != v {
			t.Errorf("expected %s=%q, got %q (present=%v)", k, v, got, ok)
		}
	}
	if len(vars) != len(want) {
		t.Errorf("expected %d variables, got %d", len(want), len(vars))
	}
}

// TestFindEnvSourceFilesMainExcludesTest проверяет поиск доп.файлов к основному
// .env: включаются .env.local/.env.dist/.env.production, а .env.test* исключаются,
// сам .env и посторонние файлы не попадают.
func TestFindEnvSourceFilesMainExcludesTest(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{".env", ".env.local", ".env.dist", ".env.test", ".env.testing", ".env.production", "other.txt"} {
		writeEnvFile(t, dir, name, "A=1\n")
	}
	files := findEnvSourceFiles(dir, ".env", true)
	names := make(map[string]bool)
	for _, f := range files {
		names[filepath.Base(f)] = true
	}
	for _, want := range []string{".env.local", ".env.dist", ".env.production"} {
		if !names[want] {
			t.Errorf("expected %s to be included", want)
		}
	}
	for _, notWant := range []string{".env", ".env.test", ".env.testing", "other.txt"} {
		if names[notWant] {
			t.Errorf("expected %s to be excluded", notWant)
		}
	}
}

// TestFindEnvSourceFilesTest проверяет поиск доп.файлов к .env.test:
// включаются .env.testing и .env.test.local, сам .env.test и .env.local — нет.
func TestFindEnvSourceFilesTest(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{".env.test", ".env.testing", ".env.test.local", ".env.local"} {
		writeEnvFile(t, dir, name, "A=1\n")
	}
	files := findEnvSourceFiles(dir, ".env.test", false)
	names := make(map[string]bool)
	for _, f := range files {
		names[filepath.Base(f)] = true
	}
	for _, want := range []string{".env.testing", ".env.test.local"} {
		if !names[want] {
			t.Errorf("expected %s to be included", want)
		}
	}
	for _, notWant := range []string{".env.test", ".env.local"} {
		if names[notWant] {
			t.Errorf("expected %s to be excluded", notWant)
		}
	}
}

// TestBuildEnvSyncActions проверяет построение действий синхронизации:
// когда все переменные источника уже есть в целевом файле — статус StatusDone,
// когда в целевом файле чего-то не хватает — StatusPending, а после запуска
// действия недостающие переменные переносятся без перезаписи существующих.
func TestBuildEnvSyncActions(t *testing.T) {
	dir := t.TempDir()
	target := writeEnvFile(t, dir, ".env", "APP_ENV=prod\nEXISTS=1\n")
	srcAllPresent := writeEnvFile(t, dir, ".env.local", "APP_ENV=prod\n")
	srcMissing := writeEnvFile(t, dir, ".env.dist", "APP_ENV=prod\nNEW_VAR=hello\n")

	actions := buildEnvSyncActions(target, []string{srcAllPresent, srcMissing})
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[0].Status != StatusDone {
		t.Errorf("expected action[0] StatusDone, got %v", actions[0].Status)
	}
	if actions[1].Status != StatusPending {
		t.Errorf("expected action[1] StatusPending, got %v", actions[1].Status)
	}
	// Выполняем перенос недостающих переменных
	if err := actions[1].Run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	vars, err := parseEnvVariables(target)
	if err != nil {
		t.Fatalf("failed to parse target: %v", err)
	}
	if vars["NEW_VAR"] != "hello" {
		t.Errorf("expected NEW_VAR=hello after transfer, got %q", vars["NEW_VAR"])
	}
	if vars["APP_ENV"] != "prod" {
		t.Errorf("expected APP_ENV unchanged, got %q", vars["APP_ENV"])
	}
}

// TestMergeEnvFileNoOverwrite проверяет, что mergeEnvFile добавляет только
// недостающие переменные и не перезаписывает существующие значения.
func TestMergeEnvFileNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := writeEnvFile(t, dir, ".env", "KEY=old\n")
	src := writeEnvFile(t, dir, ".env.dist", "KEY=new\nONLY_SRC=1\n")
	if err := mergeEnvFile(target, src); err != nil {
		t.Fatalf("mergeEnvFile failed: %v", err)
	}
	vars, err := parseEnvVariables(target)
	if err != nil {
		t.Fatalf("failed to parse target: %v", err)
	}
	if vars["KEY"] != "old" {
		t.Errorf("expected KEY=old (no overwrite), got %q", vars["KEY"])
	}
	if vars["ONLY_SRC"] != "1" {
		t.Errorf("expected ONLY_SRC=1, got %q", vars["ONLY_SRC"])
	}
}
