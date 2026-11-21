package common

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile создаёт файл с содержимым, включая все родительские директории.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// inTempDir переходит во временную директорию на время теста.
func inTempDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	return tmpDir
}

// TestFindGoMainCmd проверяет поиск main файлов в директории cmd
func TestFindGoMainCmd(t *testing.T) {
	tmpDir := inTempDir(t)
	writeFile(t, filepath.Join(tmpDir, "cmd", "foo", "main.go"),
		"package main\n\nfunc main() {}\n")
	writeFile(t, filepath.Join(tmpDir, "cmd", "foo", "utils.go"), "package foo\n")
	writeFile(t, filepath.Join(tmpDir, "cmd", "bar", "main.go"), "package main\nfunc main(){}\n")

	mains, err := FindGoMain(".", FindGoMainOptions{
		SearchInCmdFirst: true,
		ExcludeDirs:      []string{"vendor/", "internal/", ".git/"},
		OnlyMainGo:       false,
	})
	if err != nil {
		t.Fatalf("FindGoMain вернула ошибку: %v", err)
	}
	expected := map[string]bool{
		"cmd/foo/main.go": true,
		"cmd/bar/main.go": true,
	}
	if len(mains) != len(expected) {
		t.Fatalf("FindGoMain вернула %d файлов, ожидалось %d: %v", len(mains), len(expected), mains)
	}
	for _, m := range mains {
		if !expected[m] {
			t.Errorf("неожиданный main файл: %s", m)
		}
	}
}

// TestFindGoMainOutsideCmd проверяет поиск main.go вне cmd
func TestFindGoMainOutsideCmd(t *testing.T) {
	tmpDir := inTempDir(t)
	writeFile(t, filepath.Join(tmpDir, "main.go"), "package main\nfunc main(){}\n")
	writeFile(t, filepath.Join(tmpDir, "vendor", "somepkg", "main.go"), "package main\nfunc main(){}\n")

	mains, err := FindGoMain(".", FindGoMainOptions{
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
	if mains[0] != "main.go" {
		t.Errorf("ожидался main.go, получили %s", mains[0])
	}
}

// TestFindGoMainIgnoresLiterals проверяет, что строковые литералы и тестовые
// файлы не считаются точкой входа.
func TestFindGoMainIgnoresLiterals(t *testing.T) {
	tmpDir := inTempDir(t)
	// Файл содержит "package main" и "func main" только как строки.
	writeFile(t, filepath.Join(tmpDir, "helper.go"),
		"package helper\n\nvar a = \"package main\"\nvar b = \"func main()\"\n")
	// Тестовый файл не является точкой входа приложения.
	writeFile(t, filepath.Join(tmpDir, "app_test.go"), "package main\nfunc main(){}\n")

	mains, err := FindGoMain(".", FindGoMainOptions{
		SearchInCmdFirst: false,
		ExcludeDirs:      []string{},
		OnlyMainGo:       false,
	})
	if err != nil {
		t.Fatalf("FindGoMain вернула ошибку: %v", err)
	}
	if len(mains) != 0 {
		t.Fatalf("ожидалось 0 main-файлов, получили %d: %v", len(mains), mains)
	}
}

// TestFindGoMainPackages проверяет единый поиск пакетов main по директориям
func TestFindGoMainPackages(t *testing.T) {
	tmpDir := inTempDir(t)
	writeFile(t, filepath.Join(tmpDir, "cmd", "dev", "main.go"), "package main\nfunc main(){}\n")
	writeFile(t, filepath.Join(tmpDir, "cmd", "prod", "main.go"), "package main\nfunc main(){}\n")
	// Ложная точка входа: литералы в пакете internal.
	writeFile(t, filepath.Join(tmpDir, "internal", "helper", "helper.go"),
		"package helper\n\nvar a = \"package main\"\nvar b = \"func main()\"\n")

	dirs, err := FindGoMainPackages(".")
	if err != nil {
		t.Fatalf("FindGoMainPackages вернула ошибку: %v", err)
	}
	want := []string{"cmd/dev", "cmd/prod"}
	if len(dirs) != len(want) {
		t.Fatalf("FindGoMainPackages вернула %d директорий, ожидалось %d: %v", len(dirs), len(want), dirs)
	}
	for i := range want {
		if dirs[i] != want[i] {
			t.Errorf("FindGoMainPackages[%d] = %q, want %q", i, dirs[i], want[i])
		}
	}
}

// TestGoPackageArg проверяет нормализацию директории пакета для go build/go run
func TestGoPackageArg(t *testing.T) {
	tests := []struct {
		dir  string
		want string
	}{
		{".", "."},
		{"./cmd/prod", "./cmd/prod"},
		{"../other", "../other"},
		{"cmd/prod", "./cmd/prod"},
		{"/abs/pkg", "/abs/pkg"},
	}
	for _, tt := range tests {
		got := GoPackageArg(tt.dir)
		if got != tt.want {
			t.Errorf("GoPackageArg(%q) = %q, want %q", tt.dir, got, tt.want)
		}
	}
}

// TestResolveGoPackageTarget проверяет разбор явно указанной цели
func TestResolveGoPackageTarget(t *testing.T) {
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "cmd", "foo")
	mainFile := filepath.Join(pkgDir, "main.go")
	writeFile(t, mainFile, "package main\nfunc main(){}\n")
	notGo := filepath.Join(pkgDir, "README.md")
	writeFile(t, notGo, "docs")

	// .go-файл заменяется директорией пакета
	got, err := ResolveGoPackageTarget(mainFile)
	if err != nil {
		t.Fatalf("ResolveGoPackageTarget(%q) вернула ошибку: %v", mainFile, err)
	}
	if got != pkgDir {
		t.Errorf("ResolveGoPackageTarget(%q) = %q, want %q", mainFile, got, pkgDir)
	}

	// директория пакета возвращается как есть
	got, err = ResolveGoPackageTarget(pkgDir)
	if err != nil {
		t.Fatalf("ResolveGoPackageTarget(%q) вернула ошибку: %v", pkgDir, err)
	}
	if got != pkgDir {
		t.Errorf("ResolveGoPackageTarget(%q) = %q, want %q", pkgDir, got, pkgDir)
	}

	// несуществующий путь считается import path и передаётся как есть
	const importPath = "example.com/mod/cmd/foo"
	got, err = ResolveGoPackageTarget(importPath)
	if err != nil {
		t.Fatalf("ResolveGoPackageTarget(%q) вернула ошибку: %v", importPath, err)
	}
	if got != importPath {
		t.Errorf("ResolveGoPackageTarget(%q) = %q, want %q", importPath, got, importPath)
	}

	// существующий файл не .go — ошибка
	if _, err := ResolveGoPackageTarget(notGo); err == nil {
		t.Errorf("ResolveGoPackageTarget(%q) ожидалась ошибка", notGo)
	}
}

// TestSelectGoTargetExplicit проверяет, что явная цель не требует поиска
func TestSelectGoTargetExplicit(t *testing.T) {
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "cmd", "foo")
	mainFile := filepath.Join(pkgDir, "main.go")
	writeFile(t, mainFile, "package main\nfunc main(){}\n")

	got, err := SelectGoTarget(mainFile, "Select number to build [1]: ")
	if err != nil {
		t.Fatalf("SelectGoTarget вернула ошибку: %v", err)
	}
	if got != pkgDir {
		t.Errorf("SelectGoTarget(%q) = %q, want %q", mainFile, got, pkgDir)
	}
}
