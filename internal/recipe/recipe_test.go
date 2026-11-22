package recipe

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dev/internal/custom"
	"dev/internal/i18n"

	"github.com/fatih/color"
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

func TestExecutePassesArgs(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "arg.txt")
	prepared := &Prepared{
		Name: "echoarg",
		Command: custom.Command{
			Name:        "echoarg",
			Subcommands: []string{`printf '%s' "$1" > ` + out},
		},
	}

	err := Execute(prepared, custom.Context{Dir: dir, Args: []string{"8000"}})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read arg: %v", err)
	}
	if string(got) != "8000" {
		t.Errorf("arg = %q, want %q", string(got), "8000")
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

// captureOutput перехватывает stdout и color.Output на время fn и
// возвращает напечатанный текст. Язык фиксируется английским, чтобы
// проверки заголовков не зависели от окружения.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()

	i18n.SetLang("en")
	t.Cleanup(func() { i18n.SetLang("") })

	oldStdout := os.Stdout
	oldColorOutput := color.Output
	oldNoColor := color.NoColor

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	color.Output = w
	color.NoColor = true
	t.Cleanup(func() {
		os.Stdout = oldStdout
		color.Output = oldColorOutput
		color.NoColor = oldNoColor
	})

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close pipe reader: %v", err)
	}
	return string(data)
}

func TestPrintNotFoundPrintsSingleHeader(t *testing.T) {
	out := captureOutput(t, func() {
		printNotFound(&NotFoundError{
			Query:   "run",
			Matches: []ListItem{{Name: "ci.github.run"}, {Name: "ci.gitlab.run"}},
		})
	})

	if strings.Count(out, "Matching recipes:") != 1 {
		t.Errorf("output must contain exactly one \"Matching recipes:\" header:\n%s", out)
	}
	if strings.Contains(out, "Available recipes:") {
		t.Errorf("output must not contain \"Available recipes:\":\n%s", out)
	}
	for _, name := range []string{"ci.github.run", "ci.gitlab.run"} {
		if !strings.Contains(out, name) {
			t.Errorf("output does not list %q:\n%s", name, out)
		}
	}
}

func TestPrintNotFoundWithoutMatches(t *testing.T) {
	out := captureOutput(t, func() {
		printNotFound(&NotFoundError{Query: "run"})
	})

	if !strings.Contains(out, `No recipes match "run".`) {
		t.Errorf("output does not contain the no-match message:\n%s", out)
	}
	if strings.Contains(out, "Matching recipes:") || strings.Contains(out, "Available recipes:") {
		t.Errorf("output must not contain list headers when there are no matches:\n%s", out)
	}
}

func TestPrintListPrintsSingleHeader(t *testing.T) {
	out := captureOutput(t, func() {
		printList([]ListItem{{Name: "deploy", Description: "Deploy the project"}})
	})

	if strings.Count(out, "Available recipes:") != 1 {
		t.Errorf("output must contain exactly one \"Available recipes:\" header:\n%s", out)
	}
	if !strings.Contains(out, "deploy") || !strings.Contains(out, "Deploy the project") {
		t.Errorf("output does not list the recipe with its description:\n%s", out)
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

// unsetenv убирает переменную окружения на время теста и восстанавливает её.
func unsetenv(t *testing.T, key string) {
	t.Helper()
	old, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}

// cachedRecipe записывает файл рецепта в кэш пользователя.
func cachedRecipe(t *testing.T, home, name, content string) {
	t.Helper()
	writeFile(t, filepath.Join(home, "dev-command", RecipesDir, name+".yml"), content)
}

func TestPrepareParsesParams(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cachedRecipe(t, home, "deploy", `commands:
  deploy:
    description: Deploy
    params:
      - name: DEPLOY_ENV
        description: Target environment
        default: staging
      - name: PORT
    subcommands:
      - echo deploy
`)

	prepared, err := Prepare("deploy", false)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	params := prepared.Command.Params
	if len(params) != 2 {
		t.Fatalf("Params = %+v, want 2 parameters", params)
	}
	first := params[0]
	if first.Name != "DEPLOY_ENV" || first.Description != "Target environment" || first.Default != "staging" {
		t.Errorf("first param = %+v, want DEPLOY_ENV with description and default", first)
	}
	if params[1].Name != "PORT" || params[1].Default != "" {
		t.Errorf("second param = %+v, want PORT without default", params[1])
	}
}

func TestParseRecipeRejectsInvalidParamName(t *testing.T) {
	i18n.SetLang("en")
	t.Cleanup(func() { i18n.SetLang("") })

	_, err := parseRecipe([]byte(`commands:
  deploy:
    params:
      - name: "1BAD"
    subcommands:
      - echo deploy
`), "deploy")
	if err == nil {
		t.Fatal("parseRecipe() should fail for an invalid parameter name")
	}
	if !strings.Contains(err.Error(), "invalid parameter name") {
		t.Errorf("error = %v, want invalid parameter name", err)
	}
}

func TestParseRecipeRejectsDuplicateParam(t *testing.T) {
	i18n.SetLang("en")
	t.Cleanup(func() { i18n.SetLang("") })

	_, err := parseRecipe([]byte(`commands:
  deploy:
    params:
      - name: DEPLOY_ENV
      - name: DEPLOY_ENV
    subcommands:
      - echo deploy
`), "deploy")
	if err == nil {
		t.Fatal("parseRecipe() should fail for a duplicate parameter")
	}
	if !strings.Contains(err.Error(), "duplicate parameter") {
		t.Errorf("error = %v, want duplicate parameter", err)
	}
}

func TestExecutePassesEnv(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "env.txt")
	prepared := &Prepared{
		Name: "deploy",
		Command: custom.Command{
			Name:        "deploy",
			Subcommands: []string{`printf '%s' "$DEPLOY_ENV" > ` + out},
		},
	}

	err := Execute(prepared, custom.Context{Dir: dir, Env: map[string]string{"DEPLOY_ENV": "staging"}})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	if string(got) != "staging" {
		t.Errorf("env value = %q, want staging", string(got))
	}
}

func TestRunPromptsParamsAndExecutes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	out := filepath.Join(t.TempDir(), "env.txt")
	cachedRecipe(t, home, "deploy", `commands:
  deploy:
    params:
      - name: DEPLOY_ENV
        description: Target environment
    subcommands:
      - printf '%s' "$DEPLOY_ENV" > `+out+`
`)
	unsetenv(t, "DEPLOY_ENV")

	// Ввод: подтверждение запуска, затем значение параметра.
	if err := run([]string{"deploy"}, false, strings.NewReader("y\nprod\n")); err != nil {
		t.Fatalf("run() error: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	if string(got) != "prod" {
		t.Errorf("env value = %q, want prod", string(got))
	}
}
