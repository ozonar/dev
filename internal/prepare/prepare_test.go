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
