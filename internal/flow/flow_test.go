package flow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitRun выполняет git в текущей директории теста и возвращает вывод.
func gitRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := gitTry(args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(out)
}

// gitTry выполняет git и возвращает объединённый вывод вместе с ошибкой.
func gitTry(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// writeFile записывает содержимое файла в текущей директории теста.
func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// readFile читает файл в текущей директории теста.
func readFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// newRepo создаёт временный git-репозиторий с одним коммитом в ветке main
// и делает его текущей директорией теста.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	gitRun(t, "init", "-q", "-b", "main")
	gitRun(t, "config", "user.email", "me@example.com")
	gitRun(t, "config", "user.name", "Me")
	writeFile(t, "file.txt", "one\n")
	gitRun(t, "add", ".")
	gitRun(t, "commit", "-q", "-m", "initial")
	return dir
}

// commitAs создаёт коммит от указанного автора.
func commitAs(t *testing.T, email, message string) {
	t.Helper()
	gitRun(t, "-c", "user.email="+email, "-c", "user.name=Other",
		"commit", "-q", "-am", message)
}

// TestRecentBranchesFiltersRootsAndSortsMine проверяет, что корневые ветки
// не попадают в список, а ветки текущего пользователя идут первыми.
func TestRecentBranchesFiltersRootsAndSortsMine(t *testing.T) {
	newRepo(t)

	// Корневые ветки: develop и release.
	gitRun(t, "branch", "develop")
	gitRun(t, "branch", "release/1.0")

	// Моя ветка с моим коммитом.
	gitRun(t, "switch", "-q", "-c", "feature/mine")
	writeFile(t, "mine.txt", "mine\n")
	gitRun(t, "add", ".")
	gitRun(t, "commit", "-q", "-m", "my work")

	// Чужая ветка с коммитом другого автора.
	gitRun(t, "switch", "-q", "-c", "feature/other", "main")
	writeFile(t, "other.txt", "other\n")
	gitRun(t, "add", ".")
	commitAs(t, "other@example.com", "other work")
	gitRun(t, "switch", "-q", "main")

	branches, err := listLocalBranches()
	if err != nil {
		t.Fatalf("listLocalBranches: %v", err)
	}
	recent := recentBranches(branches, "main", time.Now().AddDate(0, 0, -30), 20)

	var names []string
	for _, b := range recent {
		names = append(names, b.Name)
	}
	if len(recent) != 2 {
		t.Fatalf("recent branches = %v, want feature/mine and feature/other", names)
	}
	if recent[0].Name != "feature/mine" || !recent[0].Mine {
		t.Errorf("first branch = %+v, want my own feature/mine", recent[0])
	}
	for _, b := range recent {
		if isRootBranch(b.Name, "main") {
			t.Errorf("root branch %s must be excluded", b.Name)
		}
	}
}

// TestStashRoundTrip проверяет сохранение и восстановление изменений.
func TestStashRoundTrip(t *testing.T) {
	newRepo(t)
	writeFile(t, "file.txt", "changed\n")
	writeFile(t, "new.txt", "untracked\n")

	stash, stashed, err := pushStash("feature/x", "main")
	if err != nil {
		t.Fatalf("pushStash: %v", err)
	}
	if !stashed {
		t.Fatal("pushStash must save existing changes")
	}
	if dirty, _ := worktreeDirty(); dirty {
		t.Fatal("worktree must be clean after stash")
	}
	if !stashMatches(stash.Message, "feature/x") {
		t.Errorf("stash message %q does not match feature/x", stash.Message)
	}
	if stashMatches(stash.Message, "feature") {
		t.Errorf("stash message %q must not match feature", stash.Message)
	}

	restoreStash("feature/x")
	if got := readFile(t, "file.txt"); got != "changed\n" {
		t.Errorf("file.txt = %q, want restored content", got)
	}
	if got := readFile(t, "new.txt"); got != "untracked\n" {
		t.Errorf("new.txt = %q, want restored untracked file", got)
	}
	if out := gitRun(t, "stash", "list"); out != "" {
		t.Errorf("stash list must be empty after restore, got %q", out)
	}
}

// TestRunSwitchExistingPreservesWork проверяет перенос незакоммиченных
// изменений при переключении на существующую ветку.
func TestRunSwitchExistingPreservesWork(t *testing.T) {
	newRepo(t)
	gitRun(t, "switch", "-q", "-c", "feature")
	writeFile(t, "feature.txt", "feature\n")
	gitRun(t, "add", ".")
	gitRun(t, "commit", "-q", "-m", "feature work")
	gitRun(t, "switch", "-q", "main")

	writeFile(t, "file.txt", "main change\n")
	if err := Run(Options{Branch: "feature"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if branch := gitRun(t, "branch", "--show-current"); branch != "feature" {
		t.Fatalf("current branch = %s, want feature", branch)
	}
	if got := readFile(t, "file.txt"); got != "main change\n" {
		t.Errorf("file.txt = %q, want change carried over", got)
	}
	if out := gitRun(t, "stash", "list"); out != "" {
		t.Errorf("stash must be empty, got %q", out)
	}
}

// TestRunCreatesBranchWithoutUpstream проверяет создание новой ветки от
// локальной базы и отсутствие upstream у неё.
func TestRunCreatesBranchWithoutUpstream(t *testing.T) {
	newRepo(t)
	if err := Run(Options{Branch: "feature/new", From: "main"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if branch := gitRun(t, "branch", "--show-current"); branch != "feature/new" {
		t.Fatalf("current branch = %s, want feature/new", branch)
	}
	if _, err := gitTry("rev-parse", "--abbrev-ref", "feature/new@{upstream}"); err == nil {
		t.Error("new branch must not have an upstream")
	}
}

// TestRunCreatesBranchFromRemoteBase проверяет обновление базы из origin
// и ветвление от удалённой ветки.
func TestRunCreatesBranchFromRemoteBase(t *testing.T) {
	newRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, "init", "-q", "--bare", remote)
	gitRun(t, "remote", "add", "origin", remote)
	gitRun(t, "push", "-q", "-u", "origin", "main")

	// develop существует только на origin.
	gitRun(t, "branch", "develop")
	gitRun(t, "push", "-q", "origin", "develop")
	gitRun(t, "branch", "-D", "develop")

	if err := Run(Options{Branch: "feature/from-remote", From: "develop"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if branch := gitRun(t, "branch", "--show-current"); branch != "feature/from-remote" {
		t.Fatalf("current branch = %s, want feature/from-remote", branch)
	}
	if out, err := gitTry("rev-parse", "--abbrev-ref", "feature/from-remote@{upstream}"); err == nil {
		t.Errorf("new branch must not track origin/develop, got %q", out)
	}
	// База должна совпадать с origin/develop.
	base := gitRun(t, "rev-parse", "feature/from-remote")
	want := gitRun(t, "rev-parse", "origin/develop")
	if base != want {
		t.Errorf("branch base = %s, want origin/develop %s", base, want)
	}
}

// TestRunCopyAllCommits проверяет перенос коммитов и незакоммиченных изменений.
func TestRunCopyAllCommits(t *testing.T) {
	newRepo(t)
	gitRun(t, "switch", "-q", "-c", "work")
	writeFile(t, "one.txt", "one\n")
	gitRun(t, "add", ".")
	gitRun(t, "commit", "-q", "-m", "work 1")
	writeFile(t, "two.txt", "two\n")
	gitRun(t, "add", ".")
	gitRun(t, "commit", "-q", "-m", "work 2")
	writeFile(t, "file.txt", "pending\n")

	if err := RunCopy(CopyOptions{
		Options: Options{Branch: "copy", From: "main"},
		Changes: CopyAll,
	}); err != nil {
		t.Fatalf("RunCopy: %v", err)
	}
	if branch := gitRun(t, "branch", "--show-current"); branch != "copy" {
		t.Fatalf("current branch = %s, want copy", branch)
	}
	log := gitRun(t, "log", "--format=%s")
	for _, want := range []string{"work 1", "work 2"} {
		if !strings.Contains(log, want) {
			t.Errorf("log does not contain %q:\n%s", want, log)
		}
	}
	if got := readFile(t, "file.txt"); got != "pending\n" {
		t.Errorf("file.txt = %q, want uncommitted change carried over", got)
	}
	if out := gitRun(t, "stash", "list"); out != "" {
		t.Errorf("stash must be empty, got %q", out)
	}
}

// TestRunCopyUncommitted проверяет режим переноса только незакоммиченных
// изменений: коммиты текущей ветки в новую не попадают.
func TestRunCopyUncommitted(t *testing.T) {
	newRepo(t)
	gitRun(t, "switch", "-q", "-c", "work")
	writeFile(t, "committed.txt", "committed\n")
	gitRun(t, "add", ".")
	gitRun(t, "commit", "-q", "-m", "committed work")
	writeFile(t, "file.txt", "pending\n")

	if err := RunCopy(CopyOptions{Options: Options{Branch: "copy", From: "main"}}); err != nil {
		t.Fatalf("RunCopy: %v", err)
	}
	log := gitRun(t, "log", "--format=%s")
	if strings.Contains(log, "committed work") {
		t.Errorf("uncommitted mode must not copy commits:\n%s", log)
	}
	if got := readFile(t, "file.txt"); got != "pending\n" {
		t.Errorf("file.txt = %q, want uncommitted change", got)
	}
}

// TestRunExistingBranchDeclined проверяет отказ от переключения на
// существующую ветку: работа остаётся на месте, стэш не создаётся.
func TestRunExistingBranchDeclined(t *testing.T) {
	newRepo(t)
	gitRun(t, "switch", "-q", "-c", "feature")
	gitRun(t, "switch", "-q", "main")
	writeFile(t, "file.txt", "keep me\n")

	// Имя ветки вводится текстом, затем отказ на подтверждении.
	in := strings.NewReader("feature\nn\n")
	if err := Run(Options{In: in}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if branch := gitRun(t, "branch", "--show-current"); branch != "main" {
		t.Errorf("current branch = %s, want main", branch)
	}
	if got := readFile(t, "file.txt"); got != "keep me\n" {
		t.Errorf("file.txt = %q, work must stay untouched", got)
	}
	if out := gitRun(t, "stash", "list"); out != "" {
		t.Errorf("stash must be empty, got %q", out)
	}
}

// TestParseCopyChanges проверяет разбор флага --changes.
func TestParseCopyChanges(t *testing.T) {
	cases := []struct {
		in   string
		want CopyChanges
		ok   bool
	}{
		{"", CopyUncommitted, true},
		{"uncommitted", CopyUncommitted, true},
		{"ALL", CopyAll, true},
		{"all", CopyAll, true},
		{"everything", 0, false},
	}
	for _, c := range cases {
		got, err := ParseCopyChanges(c.in)
		if c.ok {
			if err != nil || got != c.want {
				t.Errorf("ParseCopyChanges(%q) = %v, %v; want %v", c.in, got, err, c.want)
			}
			continue
		}
		if err == nil {
			t.Errorf("ParseCopyChanges(%q) must fail", c.in)
		}
	}
}

// TestRunSwitchRemoteOnlyBranch проверяет переключение на ветку, которая
// существует только в origin: создаётся локальная ветка с upstream.
func TestRunSwitchRemoteOnlyBranch(t *testing.T) {
	newRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, "init", "-q", "--bare", remote)
	gitRun(t, "remote", "add", "origin", remote)
	gitRun(t, "push", "-q", "-u", "origin", "main")

	gitRun(t, "branch", "feature/remote")
	gitRun(t, "push", "-q", "origin", "feature/remote")
	gitRun(t, "branch", "-D", "feature/remote")

	if err := Run(Options{Branch: "feature/remote"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if branch := gitRun(t, "branch", "--show-current"); branch != "feature/remote" {
		t.Fatalf("current branch = %s, want feature/remote", branch)
	}
	if up := gitRun(t, "rev-parse", "--abbrev-ref", "feature/remote@{upstream}"); up != "origin/feature/remote" {
		t.Errorf("upstream = %s, want origin/feature/remote", up)
	}
}

// TestBaseCandidatesAndDefault проверяет порядок кандидатов и выбор базы.
func TestBaseCandidatesAndDefault(t *testing.T) {
	names := []string{"main", "develop", "release/2.0", "release/1.0", "feature/x"}
	candidates := baseCandidates(names, "main")
	want := []string{"develop", "main", "release/1.0", "release/2.0"}
	if strings.Join(candidates, ",") != strings.Join(want, ",") {
		t.Fatalf("baseCandidates = %v, want %v", candidates, want)
	}
	if got := defaultBase("main", candidates, true); got != "develop" {
		t.Errorf("defaultBase(preferDevelop) = %s, want develop", got)
	}
	if got := defaultBase("main", candidates, false); got != "main" {
		t.Errorf("defaultBase(no develop) = %s, want main", got)
	}
}
