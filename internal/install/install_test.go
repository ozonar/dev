package install

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestChooseInstallDirNonInteractiveSystem проверяет, что в неинтерактивном
// режиме выбирается первый доступный кандидат — системная директория.
func TestChooseInstallDirNonInteractiveSystem(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("checkPathDirs uses ':' as PATH separator")
	}

	home := t.TempDir()
	userBin := filepath.Join(home, ".local", "bin")
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/local/bin:/usr/bin:"+userBin)

	got, err := chooseInstallDir(true)
	if err != nil {
		t.Fatalf("chooseInstallDir() error = %v", err)
	}
	if want := "/usr/local/bin"; got != want {
		t.Fatalf("chooseInstallDir() = %q, want %q", got, want)
	}
}

// TestChooseInstallDirNonInteractiveUser проверяет, что при отсутствии
// системных кандидатов в неинтерактивном режиме выбирается пользовательская
// директория.
func TestChooseInstallDirNonInteractiveUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("checkPathDirs uses ':' as PATH separator")
	}

	home := t.TempDir()
	userBin := filepath.Join(home, ".local", "bin")
	t.Setenv("HOME", home)
	t.Setenv("PATH", userBin+":/nonexistent")

	got, err := chooseInstallDir(true)
	if err != nil {
		t.Fatalf("chooseInstallDir() error = %v", err)
	}
	if got != userBin {
		t.Fatalf("chooseInstallDir() = %q, want %q", got, userBin)
	}
}

// TestChooseInstallDirNonInteractiveDoesNotReadStdin проверяет, что
// неинтерактивный режим не обращается к stdin.
func TestChooseInstallDirNonInteractiveDoesNotReadStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("checkPathDirs uses ':' as PATH separator")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer func() { _ = r.Close() }()
	// Закрываем пишущий конец: любое чтение получит EOF, а не заблокируется,
	// поэтому тест не зависнет даже при регрессии.
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	got, err := chooseInstallDir(true)
	if err != nil {
		t.Fatalf("chooseInstallDir() error = %v", err)
	}
	if got != "/usr/bin" {
		t.Fatalf("chooseInstallDir() = %q, want %q", got, "/usr/bin")
	}
}
