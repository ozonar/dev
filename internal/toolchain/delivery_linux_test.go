//go:build linux

package toolchain

import "testing"

// TestLinuxAmd64 проверяет правила доставки для linux/amd64.
func TestLinuxAmd64(t *testing.T) {
	d := linuxAmd64{}
	if got := d.PhpBinaryName("8.3"); got != "usr/bin/php8.3" {
		t.Errorf("PhpBinaryName = %q", got)
	}
	if got := d.BiomeAsset(); got != "biome-linux-x64" {
		t.Errorf("BiomeAsset = %q", got)
	}
	if got := d.RuffTarget(); got != "x86_64-unknown-linux-gnu" {
		t.Errorf("RuffTarget = %q", got)
	}
	src, ok := d.PhpSource().(linuxPhpSource)
	if !ok {
		t.Fatalf("PhpSource type = %T, want linuxPhpSource", d.PhpSource())
	}
	if src.archSuffix != "" {
		t.Errorf("linux/amd64 archSuffix = %q, want empty", src.archSuffix)
	}
}

// TestLinuxArm64 проверяет правила доставки для linux/arm64.
func TestLinuxArm64(t *testing.T) {
	d := linuxArm64{}
	if got := d.PhpBinaryName("8.3"); got != "usr/bin/php8.3" {
		t.Errorf("PhpBinaryName = %q", got)
	}
	if got := d.BiomeAsset(); got != "biome-linux-arm64" {
		t.Errorf("BiomeAsset = %q", got)
	}
	if got := d.RuffTarget(); got != "aarch64-unknown-linux-gnu" {
		t.Errorf("RuffTarget = %q", got)
	}
	src, ok := d.PhpSource().(linuxPhpSource)
	if !ok {
		t.Fatalf("PhpSource type = %T, want linuxPhpSource", d.PhpSource())
	}
	if src.archSuffix != "_arm64" {
		t.Errorf("linux/arm64 archSuffix = %q, want _arm64", src.archSuffix)
	}
}

// TestCurrentDeliveryIsLinux проверяет, что на Linux активен linux-адаптер.
func TestCurrentDeliveryIsLinux(t *testing.T) {
	d := CurrentDelivery()
	switch d.(type) {
	case linuxAmd64, linuxArm64:
		// ок — платформа зарегистрирована
	default:
		t.Fatalf("CurrentDelivery on linux = %T, want linuxAmd64/linuxArm64", d)
	}
}
