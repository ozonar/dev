//go:build darwin

package toolchain

import "testing"

// TestDarwinAmd64 проверяет правила доставки для darwin/amd64.
func TestDarwinAmd64(t *testing.T) {
	d := darwinAmd64{}
	if got := d.PhpBinaryName("8.3"); got != "php" {
		t.Errorf("PhpBinaryName = %q", got)
	}
	if got := d.BiomeAsset(); got != "biome-darwin-x64" {
		t.Errorf("BiomeAsset = %q", got)
	}
	if got := d.RuffTarget(); got != "x86_64-apple-darwin" {
		t.Errorf("RuffTarget = %q", got)
	}
	src, ok := d.PhpSource().(darwinPhpSource)
	if !ok {
		t.Fatalf("PhpSource type = %T, want darwinPhpSource", d.PhpSource())
	}
	if src.arch != "x86_64" {
		t.Errorf("darwin/amd64 arch = %q, want x86_64", src.arch)
	}
}

// TestDarwinArm64 проверяет правила доставки для darwin/arm64.
func TestDarwinArm64(t *testing.T) {
	d := darwinArm64{}
	if got := d.PhpBinaryName("8.3"); got != "php" {
		t.Errorf("PhpBinaryName = %q", got)
	}
	if got := d.BiomeAsset(); got != "biome-darwin-arm64" {
		t.Errorf("BiomeAsset = %q", got)
	}
	if got := d.RuffTarget(); got != "aarch64-apple-darwin" {
		t.Errorf("RuffTarget = %q", got)
	}
	src, ok := d.PhpSource().(darwinPhpSource)
	if !ok {
		t.Fatalf("PhpSource type = %T, want darwinPhpSource", d.PhpSource())
	}
	if src.arch != "aarch64" {
		t.Errorf("darwin/arm64 arch = %q, want aarch64", src.arch)
	}
}

// TestCurrentDeliveryIsDarwin проверяет, что на macOS активен darwin-адаптер.
func TestCurrentDeliveryIsDarwin(t *testing.T) {
	d := CurrentDelivery()
	switch d.(type) {
	case darwinAmd64, darwinArm64:
		// ок — платформа зарегистрирована
	default:
		t.Fatalf("CurrentDelivery on darwin = %T, want darwinAmd64/darwinArm64", d)
	}
}
