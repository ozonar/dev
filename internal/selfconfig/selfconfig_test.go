package selfconfig

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig создаёт ~/dev-command/main.conf во временном HOME.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "dev-command", "main.conf")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return home
}

func TestPathsUsesDevCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	paths := Paths()
	want := filepath.Join(home, "dev-command", "main.conf")
	if paths[0] != want {
		t.Errorf("Paths()[0] = %q, want %q", paths[0], want)
	}
	if paths[1] != EtcPath {
		t.Errorf("Paths()[1] = %q, want %q", paths[1], EtcPath)
	}
}

func TestValueEnvOverridesFile(t *testing.T) {
	writeConfig(t, "recipe-repo=/tmp/from-file\n")
	t.Setenv("RECIPE_REPO", "/tmp/from-env")

	if got := Value("RECIPE_REPO"); got != "/tmp/from-env" {
		t.Errorf("Value() = %q, want env value", got)
	}
}

func TestValueReadsFile(t *testing.T) {
	writeConfig(t, "# comment\nrecipe-repo=/tmp/from-file\n")

	if got := Value("recipe-repo"); got != "/tmp/from-file" {
		t.Errorf("Value() = %q, want /tmp/from-file", got)
	}
}

func TestValueMissingReturnsEmpty(t *testing.T) {
	writeConfig(t, "recipe-repo=/tmp/from-file\n")

	if got := Value("missing"); got != "" {
		t.Errorf("Value(missing) = %q, want empty", got)
	}
}

func TestDirFollowsActiveConfig(t *testing.T) {
	home := writeConfig(t, "recipe-repo=/tmp/recipes\n")

	want := filepath.Join(home, "dev-command")
	if got := Dir(); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestDirWithoutConfigUsesUserDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := filepath.Join(home, "dev-command")
	if got := Dir(); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestApplyEnvOverridesKnownValues(t *testing.T) {
	t.Setenv("RECIPE_REPO", "/tmp/from-env")
	values := ApplyEnv(map[string]string{
		"RECIPE_REPO": "/tmp/from-file",
		"OTHER":       "keep",
	})

	if values["RECIPE_REPO"] != "/tmp/from-env" {
		t.Errorf("ApplyEnv()[RECIPE_REPO] = %q, want env value", values["RECIPE_REPO"])
	}
	if values["OTHER"] != "keep" {
		t.Errorf("ApplyEnv()[OTHER] = %q, want keep", values["OTHER"])
	}
}

func TestReadFileIgnoresCommentsAndEmptyLines(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "main.conf")
	if err := os.WriteFile(path, []byte("# comment\n\nA=1\nB = 2\ninvalid\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	values, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if values["A"] != "1" || values["B"] != "2" {
		t.Errorf("ReadFile() = %v, want A=1 and B=2", values)
	}
	if _, ok := values["invalid"]; ok {
		t.Error("ReadFile() parsed a line without '='")
	}
}
