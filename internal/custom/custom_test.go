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
