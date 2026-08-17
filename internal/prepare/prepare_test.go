//go:build !windows

package prepare

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsSymfonyCAInstalledNotInstalled проверяет, что когда сертификат
// не установлен, функция isSymfonyCAInstalled возвращает false.
func TestIsSymfonyCAInstalledNotInstalled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if isSymfonyCAInstalled() {
		t.Fatal("expected isSymfonyCAInstalled() to be false when no CA is installed")
	}
}

// TestIsSymfonyCAInstalledModernPath проверяет определение сертификата
// в современной директории ~/.config/symfony-cli/certs.
func TestIsSymfonyCAInstalledModernPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := filepath.Join(tmp, ".config", "symfony-cli", "certs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create cert dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "default.p12"), []byte("cert"), 0644); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	if !isSymfonyCAInstalled() {
		t.Fatal("expected isSymfonyCAInstalled() to be true for modern cert path")
	}
}

// TestIsSymfonyCAInstalledLegacyPath проверяет определение сертификата
// в legacy директории ~/.symfony5/certs.
func TestIsSymfonyCAInstalledLegacyPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := filepath.Join(tmp, ".symfony5", "certs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create cert dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "default.p12"), []byte("cert"), 0644); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	if !isSymfonyCAInstalled() {
		t.Fatal("expected isSymfonyCAInstalled() to be true for legacy cert path")
	}
}

// TestBuildActionsAddsSymfonyCAInstall проверяет, что для Symfony-проекта
// добавляется действие symfony server:ca:install, если Symfony CLI доступен в PATH.
func TestBuildActionsAddsSymfonyCAInstall(t *testing.T) {
	// Создаём фиктивный исполняемый файл symfony во временной директории
	// и добавляем её в PATH, чтобы exec.LookPath нашёл её.
	bin := t.TempDir()
	symfonyBin := filepath.Join(bin, "symfony")
	if err := os.WriteFile(symfonyBin, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("failed to create fake symfony: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	actions := buildActions("symfony", "php")
	for _, a := range actions {
		if a.Name == "symfony server:ca:install" {
			return // действие найдено
		}
	}
	t.Fatal("expected buildActions to include 'symfony server:ca:install' action")
}

// TestIsChmodSetTrue проверяет, что isChmodSet возвращает true,
// когда все элементы директории рекурсивно имеют запрошенные права.
func TestIsChmodSetTrue(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0777); err != nil {
		t.Fatalf("failed to create sub dir: %v", err)
	}
	f := filepath.Join(sub, "file.txt")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	// Явно выставляем права 0777 на все элементы (обходим влияние umask)
	for _, p := range []string{dir, sub, f} {
		if err := os.Chmod(p, 0777); err != nil {
			t.Fatalf("failed to chmod %s: %v", p, err)
		}
	}
	if !isChmodSet(dir, 0777) {
		t.Fatal("expected isChmodSet(dir, 0777) to be true")
	}
}

// TestIsChmodSetFalseWhenNotSet проверяет, что isChmodSet возвращает false,
// если хотя бы один элемент не имеет запрошенных прав.
func TestIsChmodSetFalseWhenNotSet(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	// Корневая директория имеет права 0777, но файл — 0644
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatalf("failed to chmod dir: %v", err)
	}
	if isChmodSet(dir, 0777) {
		t.Fatal("expected isChmodSet(dir, 0777) to be false when file has 0644")
	}
}

// TestIsChmodSetDifferentMode проверяет, что isChmodSet различает запрошенные
// режимы (например, 0755 уже установлен, а 0777 — нет).
func TestIsChmodSetDifferentMode(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	// Выставляем 0755 на все элементы
	for _, p := range []string{dir, f} {
		if err := os.Chmod(p, 0755); err != nil {
			t.Fatalf("failed to chmod %s: %v", p, err)
		}
	}
	if !isChmodSet(dir, 0755) {
		t.Fatal("expected isChmodSet(dir, 0755) to be true")
	}
	if isChmodSet(dir, 0777) {
		t.Fatal("expected isChmodSet(dir, 0777) to be false when only 0755 is set")
	}
}

// TestIsChmodSetNonexistent проверяет, что для несуществующей директории
// isChmodSet возвращает false.
func TestIsChmodSetNonexistent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if isChmodSet(dir, 0777) {
		t.Fatal("expected isChmodSet() to be false for nonexistent directory")
	}
}
