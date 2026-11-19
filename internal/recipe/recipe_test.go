package recipe

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dev/internal/custom"
)

// writeFile создаёт файл с содержимым, создавая родительские папки.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// localSource создаёт локальный репозиторий рецептов с одним рецептом deploy.
func localSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, IndexFile), `recipes:
  deploy:
    description: Deploy the project
    file: recipes/deploy.yml
`)
	writeFile(t, filepath.Join(dir, RecipesDir, "deploy.yml"), `commands:
  deploy:
    description: Deploy the project
    subcommands:
      - echo deploy
`)
	return dir
}

func TestRepoURLDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("RECIPE_REPO", "")
	if got := RepoURL(); got != DefaultRepo {
		t.Errorf("RepoURL() = %q, want %q", got, DefaultRepo)
	}
}

func TestRepoURLFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RECIPE_REPO", "")
	writeFile(t, filepath.Join(home, "dev-command", "main.conf"), "RECIPE_REPO=/tmp/recipes\n")

	if got := RepoURL(); got != "/tmp/recipes" {
		t.Errorf("RepoURL() = %q, want /tmp/recipes", got)
	}
}

func TestRepoURLMixedCase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RECIPE_REPO", "")
	writeFile(t, filepath.Join(home, "dev-command", "main.conf"), "reCIPE_rePO=/tmp/mixed\n")

	if got := RepoURL(); got != "/tmp/mixed" {
		t.Errorf("RepoURL() = %q, want /tmp/mixed", got)
	}
}

func TestRepoURLEnvOverridesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, "dev-command", "main.conf"), "RECIPE_REPO=/tmp/from-file\n")
	t.Setenv("RECIPE_REPO", "/tmp/from-env")

	if got := RepoURL(); got != "/tmp/from-env" {
		t.Errorf("RepoURL() = %q, want /tmp/from-env", got)
	}
}

func TestCacheDirFollowsConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, "dev-command", "main.conf"), "RECIPE_REPO=/tmp/recipes\n")

	want := filepath.Join(home, "dev-command", RecipesDir)
	if got := CacheDir(); got != want {
		t.Errorf("CacheDir() = %q, want %q", got, want)
	}
}

func TestLoadRepositoryLocal(t *testing.T) {
	source := localSource(t)
	t.Setenv("RECIPE_REPO", source)

	repo, err := LoadRepository()
	if err != nil {
		t.Fatalf("LoadRepository() error: %v", err)
	}
	entry, ok := repo.Recipes["deploy"]
	if !ok {
		t.Fatal("LoadRepository() did not parse deploy")
	}
	if entry.Description != "Deploy the project" || entry.File != "recipes/deploy.yml" {
		t.Errorf("entry = %+v, want description and file", entry)
	}
}

func TestMatchIsCaseInsensitive(t *testing.T) {
	repo := &Repository{Recipes: map[string]Entry{
		"deploy":     {Description: "Deploy"},
		"deploy_all": {Description: "Deploy all"},
		"lint":       {Description: "Lint"},
	}}
	matches := repo.Match("DEPLOY")
	if len(matches) != 2 {
		t.Fatalf("Match(DEPLOY) returned %d items, want 2", len(matches))
	}
	if matches[0].Name != "deploy" || matches[1].Name != "deploy_all" {
		t.Errorf("Match(DEPLOY) = %+v, want sorted deploy and deploy_all", matches)
	}
}

func TestPrepareFromCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, "dev-command", RecipesDir, "deploy.yml"), `commands:
  deploy:
    description: Cached deploy
    subcommands:
      - echo cached
`)

	prepared, err := Prepare("deploy", false)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !prepared.FromCache {
		t.Error("Prepare() FromCache = false, want true")
	}
	if prepared.Description != "Cached deploy" {
		t.Errorf("Description = %q, want Cached deploy", prepared.Description)
	}
}

func TestPrepareLocalDownloadsAndCaches(t *testing.T) {
	source := localSource(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RECIPE_REPO", source)

	prepared, err := Prepare("deploy", true)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if prepared.FromCache {
		t.Error("Prepare(force) FromCache = true, want false")
	}
	if prepared.Name != "deploy" || prepared.Description != "Deploy the project" {
		t.Errorf("prepared = %+v, want deploy with description", prepared)
	}

	cachePath := filepath.Join(home, "dev-command", RecipesDir, "deploy.yml")
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("recipe was not cached: %v", err)
	}
}

func TestPrepareNotFoundReturnsMatches(t *testing.T) {
	source := localSource(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RECIPE_REPO", source)

	_, err := Prepare("dep", true)
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("Prepare() error = %v, want NotFoundError", err)
	}
	if len(notFound.Matches) != 1 || notFound.Matches[0].Name != "deploy" {
		t.Errorf("Matches = %+v, want deploy", notFound.Matches)
	}
}

func TestPrepareRejectsNameMismatch(t *testing.T) {
	source := t.TempDir()
	writeFile(t, filepath.Join(source, IndexFile), `recipes:
  deploy:
    description: Deploy
    file: recipes/other.yml
`)
	writeFile(t, filepath.Join(source, RecipesDir, "other.yml"), `commands:
  other:
    subcommands:
      - echo other
`)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RECIPE_REPO", source)

	if _, err := Prepare("deploy", true); err == nil {
		t.Error("Prepare() should fail when recipe name does not match")
	}
}

func TestValidateRelPathRejectsTraversal(t *testing.T) {
	for _, rel := range []string{"../secret", "/etc/passwd", "recipes/../../secret"} {
		if err := validateRelPath(rel); err == nil {
			t.Errorf("validateRelPath(%q) = nil, want error", rel)
		}
	}
	if err := validateRelPath("recipes/deploy.yml"); err != nil {
		t.Errorf("validateRelPath(valid) error: %v", err)
	}
}

func TestReadLineKeepsRemainingInput(t *testing.T) {
	in := strings.NewReader("1\n2\n")

	if got := readLine(in); got != "1" {
		t.Fatalf("readLine() = %q, want %q", got, "1")
	}
	if got := readLine(in); got != "2" {
		t.Fatalf("second readLine() = %q, want %q", got, "2")
	}
}

func TestReadLineWithoutNewline(t *testing.T) {
	if got := readLine(strings.NewReader("y")); got != "y" {
		t.Errorf("readLine() = %q, want %q", got, "y")
	}
	if got := readLine(strings.NewReader("")); got != "" {
		t.Errorf("readLine(empty) = %q, want empty", got)
	}
}

func TestConfirm(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"\n", true},
		{"y\n", true},
		{"YES\n", true},
		{"n\n", false},
		{"no\n", false},
		{"", true},
	}
	for _, tc := range cases {
		if got := confirm(strings.NewReader(tc.input)); got != tc.want {
			t.Errorf("confirm(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestConfirmDoesNotConsumeNextLine(t *testing.T) {
	in := strings.NewReader("y\n1\n")
	if !confirm(in) {
		t.Fatal("confirm() = false, want true")
	}
	if got := readLine(in); got != "1" {
		t.Errorf("input after confirm = %q, want %q", got, "1")
	}
}

func TestExecuteRunsSubcommands(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	prepared := &Prepared{
		Name: "deploy",
		Command: custom.Command{
			Name:        "deploy",
			Subcommands: []string{"touch " + marker},
		},
	}

	if err := Execute(prepared, custom.Context{Dir: dir}); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("marker not created: %v", err)
	}
}
