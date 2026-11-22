package custom

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// testContext возвращает контекст для тестов.
func testContext(dir string) Context {
	return Context{Dir: dir, Language: "php", Framework: "symfony"}
}

func TestExpand(t *testing.T) {
	ctx := Context{Dir: "/proj", Language: "go", Framework: "gin"}
	got := Expand("cd $(current_dir)/sub && echo $(language)-$(framework)", ctx)
	want := "cd /proj/sub && echo go-gin"
	if got != want {
		t.Fatalf("Expand() = %q, want %q", got, want)
	}
}

func TestHas(t *testing.T) {
	cfg := &Config{Commands: map[string]Command{
		"deploy": {Subcommands: []string{"git pull"}},
	}}
	if !cfg.Has("deploy") {
		t.Error("Has(deploy) = false, want true")
	}
	if cfg.Has("missing") {
		t.Error("Has(missing) = true, want false")
	}
}

func TestNames(t *testing.T) {
	cfg := &Config{Commands: map[string]Command{
		"deploy":      {},
		"full_deploy": {},
	}}
	got := cfg.Names()
	want := []string{"deploy", "full_deploy"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
}

func TestRunCommandNotFound(t *testing.T) {
	cfg := &Config{Commands: map[string]Command{}}
	found, err := cfg.RunCommand("nope", testContext(t.TempDir()))
	if err != nil {
		t.Fatalf("RunCommand() unexpected error: %v", err)
	}
	if found {
		t.Error("RunCommand() found = true, want false for unknown command")
	}
}

func TestRunCommandRunsSubcommands(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "deployed.txt")

	cfg := &Config{Commands: map[string]Command{
		"deploy": {Subcommands: []string{"touch " + marker}},
	}}

	found, err := cfg.RunCommand("deploy", testContext(dir))
	if err != nil {
		t.Fatalf("RunCommand() unexpected error: %v", err)
	}
	if !found {
		t.Error("RunCommand() found = false, want true")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("marker file not created: %v", err)
	}
}

func TestRunCommandStopsOnFailure(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")

	cfg := &Config{Commands: map[string]Command{
		"fail": {Subcommands: []string{
			"touch " + first,
			"exit 1",
			"touch " + second,
		}},
	}}

	found, err := cfg.RunCommand("fail", testContext(dir))
	if err == nil {
		t.Fatal("RunCommand() expected error on failing subcommand, got nil")
	}
	if !found {
		t.Error("RunCommand() found = false, want true")
	}
	if _, err := os.Stat(first); err != nil {
		t.Errorf("first subcommand did not run: %v", err)
	}
	if _, err := os.Stat(second); err == nil {
		t.Error("execution should stop on first failure; second.txt must not exist")
	}
}

func TestRunCommandPassesArgsPositionally(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")

	cfg := &Config{Commands: map[string]Command{
		"echoargs": {Subcommands: []string{`printf '%s|%s' "$1" "$2" > ` + out}},
	}}
	ctx := testContext(dir)
	ctx.Args = []string{"8000", "hello world"}

	found, err := cfg.RunCommand("echoargs", ctx)
	if err != nil {
		t.Fatalf("RunCommand() unexpected error: %v", err)
	}
	if !found {
		t.Error("RunCommand() found = false, want true")
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if string(got) != "8000|hello world" {
		t.Errorf("args = %q, want %q", string(got), "8000|hello world")
	}
}

func TestRunCommandPassesEnv(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "env.txt")

	cfg := &Config{Commands: map[string]Command{
		"deploy": {Subcommands: []string{`printf '%s' "$DEPLOY_ENV" > ` + out}},
	}}
	// Переменная процесса не должна побеждать значение из контекста.
	t.Setenv("DEPLOY_ENV", "from-process")
	ctx := testContext(dir)
	ctx.Env = map[string]string{"DEPLOY_ENV": "staging"}

	found, err := cfg.RunCommand("deploy", ctx)
	if err != nil {
		t.Fatalf("RunCommand() unexpected error: %v", err)
	}
	if !found {
		t.Error("RunCommand() found = false, want true")
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if string(got) != "staging" {
		t.Errorf("env value = %q, want %q", string(got), "staging")
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

func TestResolveParamsPrefersArgument(t *testing.T) {
	t.Setenv("DEPLOY_ENV", "from-env")
	params := []Param{{Name: "DEPLOY_ENV", Default: "from-default"}}

	values, positional, err := resolveParams(params, []string{"DEPLOY_ENV=from-arg", "8000"}, strings.NewReader(""))
	if err != nil {
		t.Fatalf("resolveParams() error: %v", err)
	}
	if values["DEPLOY_ENV"] != "from-arg" {
		t.Errorf("value = %q, want from-arg", values["DEPLOY_ENV"])
	}
	if len(positional) != 1 || positional[0] != "8000" {
		t.Errorf("positional = %v, want [8000]", positional)
	}
}

func TestResolveParamsUsesEnvironment(t *testing.T) {
	t.Setenv("DEPLOY_ENV", "from-env")

	values, _, err := resolveParams([]Param{{Name: "DEPLOY_ENV"}}, nil, strings.NewReader(""))
	if err != nil {
		t.Fatalf("resolveParams() error: %v", err)
	}
	if values["DEPLOY_ENV"] != "from-env" {
		t.Errorf("value = %q, want from-env", values["DEPLOY_ENV"])
	}
}

func TestResolveParamsUsesEmptyEnvironment(t *testing.T) {
	t.Setenv("DEPLOY_ENV", "")

	values, _, err := resolveParams([]Param{{Name: "DEPLOY_ENV"}}, nil, strings.NewReader(""))
	if err != nil {
		t.Fatalf("resolveParams() error: %v", err)
	}
	if values["DEPLOY_ENV"] != "" {
		t.Errorf("value = %q, want empty value from the environment", values["DEPLOY_ENV"])
	}
}

func TestResolveParamsPromptsAndUsesDefault(t *testing.T) {
	unsetenv(t, "DEPLOY_ENV")

	values, _, err := resolveParams(
		[]Param{{Name: "DEPLOY_ENV", Default: "staging"}},
		nil,
		strings.NewReader("\n"),
	)
	if err != nil {
		t.Fatalf("resolveParams() error: %v", err)
	}
	if values["DEPLOY_ENV"] != "staging" {
		t.Errorf("value = %q, want staging", values["DEPLOY_ENV"])
	}
}

func TestResolveParamsPromptsForValue(t *testing.T) {
	unsetenv(t, "DEPLOY_ENV")

	values, _, err := resolveParams([]Param{{Name: "DEPLOY_ENV"}}, nil, strings.NewReader("prod\n"))
	if err != nil {
		t.Fatalf("resolveParams() error: %v", err)
	}
	if values["DEPLOY_ENV"] != "prod" {
		t.Errorf("value = %q, want prod", values["DEPLOY_ENV"])
	}
}

func TestResolveParamsRepeatsOnEmptyRequiredValue(t *testing.T) {
	unsetenv(t, "DEPLOY_ENV")

	values, _, err := resolveParams([]Param{{Name: "DEPLOY_ENV"}}, nil, strings.NewReader("\n\nprod\n"))
	if err != nil {
		t.Fatalf("resolveParams() error: %v", err)
	}
	if values["DEPLOY_ENV"] != "prod" {
		t.Errorf("value = %q, want prod", values["DEPLOY_ENV"])
	}
}

func TestResolveParamsFailsOnEOF(t *testing.T) {
	unsetenv(t, "DEPLOY_ENV")

	if _, _, err := resolveParams([]Param{{Name: "DEPLOY_ENV"}}, nil, strings.NewReader("")); err == nil {
		t.Fatal("resolveParams() should fail when a required parameter has no value")
	}
}

func TestResolveParamsKeepsUndeclaredNamedArgs(t *testing.T) {
	values, positional, err := resolveParams(
		[]Param{{Name: "DEPLOY_ENV"}},
		[]string{"OTHER=1", "DEPLOY_ENV=prod"},
		strings.NewReader(""),
	)
	if err != nil {
		t.Fatalf("resolveParams() error: %v", err)
	}
	if values["DEPLOY_ENV"] != "prod" {
		t.Errorf("value = %q, want prod", values["DEPLOY_ENV"])
	}
	if len(positional) != 1 || positional[0] != "OTHER=1" {
		t.Errorf("positional = %v, want [OTHER=1]", positional)
	}
}

func TestRunCommandPromptsParams(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "env.txt")

	cfg := &Config{Commands: map[string]Command{
		"deploy": {
			Params:      []Param{{Name: "DEPLOY_ENV", Description: "Target environment"}},
			Subcommands: []string{`printf '%s' "$DEPLOY_ENV" > ` + out},
		},
	}}
	unsetenv(t, "DEPLOY_ENV")
	ctx := testContext(dir)
	ctx.Input = strings.NewReader("prod\n")

	found, err := cfg.RunCommand("deploy", ctx)
	if err != nil {
		t.Fatalf("RunCommand() unexpected error: %v", err)
	}
	if !found {
		t.Error("RunCommand() found = false, want true")
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if string(got) != "prod" {
		t.Errorf("env value = %q, want prod", string(got))
	}
}

func TestRunCommandParamFromArgument(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "env.txt")

	cfg := &Config{Commands: map[string]Command{
		"deploy": {
			Params:      []Param{{Name: "DEPLOY_ENV"}},
			Subcommands: []string{`printf '%s' "$DEPLOY_ENV" > ` + out},
		},
	}}
	unsetenv(t, "DEPLOY_ENV")
	ctx := testContext(dir)
	ctx.Args = []string{"DEPLOY_ENV=prod"}
	// Пустой ввод: при корректной работе параметр не должен запрашиваться.
	ctx.Input = strings.NewReader("")

	if _, err := cfg.RunCommand("deploy", ctx); err != nil {
		t.Fatalf("RunCommand() unexpected error: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if string(got) != "prod" {
		t.Errorf("env value = %q, want prod", string(got))
	}
}

func TestLoadParsesParams(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	path := filepath.Join(tmp, "dev-command", "custom.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	content := `commands:
  deploy:
    params:
      - name: DEPLOY_ENV
        description: Target environment
        default: staging
    subcommands:
      - echo "$DEPLOY_ENV"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	params := cfg.Commands["deploy"].Params
	if len(params) != 1 {
		t.Fatalf("Params = %+v, want 1 parameter", params)
	}
	if params[0].Name != "DEPLOY_ENV" || params[0].Description != "Target environment" || params[0].Default != "staging" {
		t.Errorf("param = %+v, want DEPLOY_ENV with description and default", params[0])
	}
}

func TestLoadRejectsInvalidParams(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	path := filepath.Join(tmp, "dev-command", "custom.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	content := `commands:
  deploy:
    params:
      - name: "1BAD"
    subcommands:
      - echo deploy
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("Load() should reject an invalid parameter name")
	}
}

func TestRunCommandArgsAreNotShellInterpreted(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	hacked := filepath.Join(dir, "hacked.txt")

	cfg := &Config{Commands: map[string]Command{
		"echoargs": {Subcommands: []string{`printf '%s' "$1" > ` + out}},
	}}
	ctx := testContext(dir)
	arg := "8000; touch " + hacked
	ctx.Args = []string{arg}

	if _, err := cfg.RunCommand("echoargs", ctx); err != nil {
		t.Fatalf("RunCommand() unexpected error: %v", err)
	}
	if _, err := os.Stat(hacked); err == nil {
		t.Error("argument must not be interpreted by the shell")
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if string(got) != arg {
		t.Errorf("arg = %q, want %q", string(got), arg)
	}
}

func TestRunCommandCurrentDirAndCd(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	marker := filepath.Join(sub, "inside.txt")

	cfg := &Config{Commands: map[string]Command{
		"goto": {Subcommands: []string{
			"cd $(current_dir)/sub && touch " + marker,
		}},
	}}

	if _, err := cfg.RunCommand("goto", testContext(dir)); err != nil {
		t.Fatalf("RunCommand() unexpected error: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("marker not created inside sub dir: %v", err)
	}
}

func TestLoadParsesYAML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	path := filepath.Join(tmp, "dev-command", "custom.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	content := `commands:
  deploy:
    subcommands:
      - git pull
      - dev migrate
  full_deploy:
    subcommands:
      - git pull
      - composer install
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if len(cfg.Commands) != 2 {
		t.Fatalf("Load() parsed %d commands, want 2", len(cfg.Commands))
	}
	deploy := cfg.Commands["deploy"]
	if len(deploy.Subcommands) != 2 || deploy.Subcommands[0] != "git pull" || deploy.Subcommands[1] != "dev migrate" {
		t.Fatalf("deploy subcommands parsed incorrectly: %v", deploy.Subcommands)
	}
	if !strings.Contains(ConfigFilePath(), "custom.yml") {
		t.Errorf("ConfigFilePath() does not point to custom.yml: %s", ConfigFilePath())
	}
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() expected no error for missing file, got %v", err)
	}
	if cfg.Commands == nil || len(cfg.Commands) != 0 {
		t.Fatalf("Load() expected empty commands for missing file, got %v", cfg.Commands)
	}
}

func TestLoadLocalParsesYAML(t *testing.T) {
	dir := t.TempDir()
	local := `commands:
  lint:
    subcommands:
      - php-cs-fixer fix
    path: ./backend
`
	if err := os.WriteFile(filepath.Join(dir, localFileName), []byte(local), 0644); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	cfg, err := LoadLocal(dir)
	if err != nil {
		t.Fatalf("LoadLocal() unexpected error: %v", err)
	}
	lint, ok := cfg.Commands["lint"]
	if !ok {
		t.Fatal("LoadLocal() did not parse command 'lint'")
	}
	if len(lint.Subcommands) != 1 || lint.Subcommands[0] != "php-cs-fixer fix" {
		t.Fatalf("lint subcommands parsed incorrectly: %v", lint.Subcommands)
	}
	if lint.Path != "./backend" {
		t.Fatalf("lint path = %q, want %q", lint.Path, "./backend")
	}
	if LocalFilePath(dir) != filepath.Join(dir, localFileName) {
		t.Errorf("LocalFilePath() = %q, want %q", LocalFilePath(dir), filepath.Join(dir, localFileName))
	}
}

func TestLoadLocalMissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadLocal(dir)
	if err != nil {
		t.Fatalf("LoadLocal() expected no error for missing file, got %v", err)
	}
	if cfg.Commands == nil || len(cfg.Commands) != 0 {
		t.Fatalf("LoadLocal() expected empty commands for missing file, got %v", cfg.Commands)
	}
}

func TestLoadAllMergesLocalOverGlobal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Глобальный конфиг содержит общую команду и команду, которую переопределит локальный файл.
	globalPath := filepath.Join(tmp, "dev-command", "custom.yml")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0755); err != nil {
		t.Fatalf("mkdir global dir: %v", err)
	}
	global := `commands:
  deploy:
    subcommands:
      - git pull
  shared:
    subcommands:
      - echo global
`
	if err := os.WriteFile(globalPath, []byte(global), 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	// Локальный .custom переопределяет shared и добавляет lint.
	dir := t.TempDir()
	local := `commands:
  shared:
    subcommands:
      - echo local
  lint:
    subcommands:
      - php-cs-fixer fix
`
	if err := os.WriteFile(filepath.Join(dir, localFileName), []byte(local), 0644); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	cfg, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll() unexpected error: %v", err)
	}

	// Глобальная команда осталась.
	if !cfg.Has("deploy") {
		t.Error("LoadAll() lost global command 'deploy'")
	}
	// Локальная команда добавлена.
	if !cfg.Has("lint") {
		t.Error("LoadAll() lost local command 'lint'")
	}
	// Локальная версия переопределила глобальную.
	shared := cfg.Commands["shared"]
	if len(shared.Subcommands) != 1 || shared.Subcommands[0] != "echo local" {
		t.Fatalf("LoadAll() did not override 'shared', got %v", shared.Subcommands)
	}
}

func TestRunCommandRespectsPathRestriction(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "backend")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("mkdir backend: %v", err)
	}
	marker := filepath.Join(sub, "ran.txt")

	cfg := &Config{Commands: map[string]Command{
		"deploy": {
			Subcommands: []string{"touch " + marker},
			Path:        root,
		},
	}}

	// Внутри разрешённого пути команда выполняется.
	found, err := cfg.RunCommand("deploy", testContext(sub))
	if err != nil {
		t.Fatalf("RunCommand() inside path: unexpected error: %v", err)
	}
	if !found {
		t.Error("RunCommand() inside path: found = false, want true")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("marker not created inside allowed path: %v", err)
	}

	// Снаружи пути команда считается недоступной (found=false) и не выполняется.
	outside := t.TempDir()
	_ = os.Remove(marker)
	found, err = cfg.RunCommand("deploy", testContext(outside))
	if err != nil {
		t.Fatalf("RunCommand() outside path: unexpected error: %v", err)
	}
	if found {
		t.Error("RunCommand() outside path: found = true, want false")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("command executed outside the allowed path")
	}
}

func TestNamesForFiltersByPath(t *testing.T) {
	root := t.TempDir()
	cfg := &Config{Commands: map[string]Command{
		"global_only": {Subcommands: []string{"echo hi"}},
		"scoped":      {Subcommands: []string{"echo hi"}, Path: root},
	}}

	inside := NamesFor(t, cfg, testContext(root))
	if len(inside) != 2 {
		t.Fatalf("NamesFor() inside path = %v, want both commands", inside)
	}

	outside := NamesFor(t, cfg, testContext(t.TempDir()))
	if len(outside) != 1 || outside[0] != "global_only" {
		t.Fatalf("NamesFor() outside path = %v, want only [global_only]", outside)
	}
}

// NamesFor — обёртка над методом Config.NamesFor, которая добавляет
// имя метода в название теста.
func NamesFor(t *testing.T, cfg *Config, ctx Context) []string {
	t.Helper()
	return cfg.NamesFor(ctx)
}

func TestDefaultConfigTemplateIsValidYAML(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(defaultConfigTemplate), &cfg); err != nil {
		t.Fatalf("defaultConfigTemplate is not valid YAML: %v", err)
	}
	example, ok := cfg.Commands["example"]
	if !ok {
		t.Fatal("defaultConfigTemplate has no 'example' command")
	}
	if len(example.Subcommands) != 1 {
		t.Fatalf("defaultConfigTemplate example subcommands = %v, want one", example.Subcommands)
	}
}
