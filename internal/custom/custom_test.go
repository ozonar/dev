package custom

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
