package build

import (
	"dev/internal/common"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFindGoMainCmd проверяет поиск main файлов в директории cmd
func TestFindGoMainCmd(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Создаём структуру cmd/foo/main.go
	cmdDir := filepath.Join(tmpDir, "cmd", "foo")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(cmdDir, "main.go")
	content := `package main

func main() {
	println("hello")
}
`
	if err := os.WriteFile(mainFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Также создаём не-main файл
	otherFile := filepath.Join(cmdDir, "utils.go")
	if err := os.WriteFile(otherFile, []byte("package foo"), 0644); err != nil {
		t.Fatal(err)
	}

	// Создаём ещё один main в cmd/bar/main.go
	cmdBarDir := filepath.Join(tmpDir, "cmd", "bar")
	if err := os.MkdirAll(cmdBarDir, 0755); err != nil {
		t.Fatal(err)
	}
	mainBar := filepath.Join(cmdBarDir, "main.go")
	if err := os.WriteFile(mainBar, []byte("package main\nfunc main(){}"), 0644); err != nil {
		t.Fatal(err)
	}

	mains, err := common.FindGoMain(".", common.FindGoMainOptions{
		SearchInCmdFirst: true,
		ExcludeDirs:      []string{"vendor/", "internal/", ".git/"},
		OnlyMainGo:       false,
	})
	if err != nil {
		t.Fatalf("FindGoMain вернула ошибку: %v", err)
	}
	if len(mains) != 2 {
		t.Fatalf("FindGoMain вернула %d файлов, ожидалось 2: %v", len(mains), mains)
	}
	// Проверяем, что пути содержат ожидаемые имена
	expected := map[string]bool{
		"cmd/foo/main.go": true,
		"cmd/bar/main.go": true,
	}
	for _, m := range mains {
		if !expected[m] {
			t.Errorf("неожиданный main файл: %s", m)
		}
	}
}

// TestFindGoMainOutsideCmd проверяет поиск main.go вне cmd
func TestFindGoMainOutsideCmd(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Создаём main.go в корне
	mainRoot := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainRoot, []byte("package main\nfunc main(){}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Создаём vendor/main.go (должен быть проигнорирован)
	vendorDir := filepath.Join(tmpDir, "vendor", "somepkg")
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vendorDir, "main.go"), []byte("package main\nfunc main(){}"), 0644); err != nil {
		t.Fatal(err)
	}

	mains, err := common.FindGoMain(".", common.FindGoMainOptions{
		SearchInCmdFirst: true,
		ExcludeDirs:      []string{"vendor/", "internal/", ".git/"},
		OnlyMainGo:       true,
	})
	if err != nil {
		t.Fatalf("FindGoMain вернула ошибку: %v", err)
	}
	if len(mains) != 1 {
		t.Fatalf("FindGoMain вернула %d файлов, ожидался 1: %v", len(mains), mains)
	}
	if !strings.HasSuffix(mains[0], "main.go") {
		t.Errorf("ожидался main.go, получили %s", mains[0])
	}
}

// TestOutputName проверяет генерацию имени выходного файла
func TestOutputName(t *testing.T) {
	tests := []struct {
		target string
		want   string
	}{
		{"cmd/foo/main.go", "foo"},
		{"cmd/bar/baz/main.go", "bar"}, // после cmd идёт bar, а не baz
		{"main.go", "main"},
		{"app.go", "app"},
		{"cmd/subdir/another/main.go", "subdir"}, // после cmd идёт subdir
		{"cmd/foo/bar/baz/main.go", "foo"},       // после cmd идёт foo
		{"notcmd/foo/main.go", "main"},
	}
	for _, tt := range tests {
		got := outputName(tt.target)
		if got != tt.want {
			t.Errorf("outputName(%q) = %q, want %q", tt.target, got, tt.want)
		}
	}
}
