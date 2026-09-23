package toolchain

import (
	"runtime"
	"testing"
)

// TestDeliveriesRegistry проверяет, что все зарегистрированные записи реестра
// валидны, а для текущей платформы всегда находится конкретный адаптер
// (а не fallback).
func TestDeliveriesRegistry(t *testing.T) {
	for k, d := range deliveries {
		if d == nil {
			t.Errorf("nil delivery registered for %s/%s", k.goos, k.goarch)
		}
	}
	if _, isFallback := CurrentDelivery().(fallbackDelivery); isFallback {
		t.Errorf("current platform %s/%s resolved to fallback; add a delivery",
			runtime.GOOS, runtime.GOARCH)
	}
}

// TestCurrentDeliveryNotNull проверяет, что адаптер текущей платформы
// возвращает непустые имена артефактов.
func TestCurrentDeliveryNotNull(t *testing.T) {
	d := CurrentDelivery()
	if d == nil {
		t.Fatal("CurrentDelivery returned nil")
	}
	if d.BiomeAsset() == "" || d.RuffTarget() == "" {
		t.Error("CurrentDelivery must provide non-empty artifact names")
	}
}

// TestCompareFull проверяет сравнение полных версий major.minor.patch.
func TestCompareFull(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"8.4.23", "8.4.10", 1},
		{"8.4.10", "8.4.23", -1},
		{"8.4.23", "8.4.23", 0},
		{"8.4.23", "8.5.0", -1},
		{"8.5.0", "8.4.23", 1},
		{"8.4.23", "8.4", 1},
		{"8.4", "8.4.23", -1},
	}
	for _, c := range cases {
		if got := compareFull(c.a, c.b); got != c.want {
			t.Errorf("compareFull(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestSortFullVersionsDesc проверяет сортировку полных версий от новейшей
// к старейшей.
func TestSortFullVersionsDesc(t *testing.T) {
	got := sortFullVersionsDesc([]string{"8.3.5", "8.4.10", "8.4.23", "8.2.1"})
	want := []string{"8.4.23", "8.4.10", "8.3.5", "8.2.1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortFullVersionsDesc = %v, want %v", got, want)
		}
	}
}

// TestPickStaticPhpVersion проверяет выбор самой свежей подходящей версии.
func TestPickStaticPhpVersion(t *testing.T) {
	versions := []string{"8.5.8", "8.4.23", "8.4.10", "8.3.5"}

	cases := []struct {
		required, want string
	}{
		{"8.4", "8.4.23"},
		{"8.3", "8.3.5"},
		{"8.5", "8.5.8"},
		{"8.2", ""},         // нет подходящих
		{"", "8.5.8"},       // latest по умолчанию
		{"latest", "8.5.8"}, // latest явно
	}
	for _, c := range cases {
		if got := pickStaticPhpVersion(versions, c.required); got != c.want {
			t.Errorf("pickStaticPhpVersion(%q) = %q, want %q", c.required, got, c.want)
		}
	}

	if got := pickStaticPhpVersion(nil, ""); got != "" {
		t.Errorf("pickStaticPhpVersion(nil, \"\") = %q, want empty", got)
	}
}

// TestSummarizeVersions проверяет компактное описание списка версий.
func TestSummarizeVersions(t *testing.T) {
	if got := summarizeVersions([]string{"8.5.8", "8.4.23"}); got != "8.5.8, 8.4.23" {
		t.Errorf("summarizeVersions(2) = %q", got)
	}
	long := []string{"9.0.1", "8.5.8", "8.4.23", "8.3.5", "8.2.1", "8.1.0"}
	if got := summarizeVersions(long); got != "9.0.1, 8.5.8, 8.4.23, 8.3.5, 8.2.1, ..." {
		t.Errorf("summarizeVersions(6) = %q", got)
	}
}

// TestFallbackDelivery проверяет дефолтные правила для неподдерживаемых
// платформ: linux-имена линтеров, php не скачивается.
func TestFallbackDelivery(t *testing.T) {
	d := fallbackDelivery{}
	if got := d.PhpBinaryName("8.3"); got != "usr/bin/php8.3" {
		t.Errorf("fallback PhpBinaryName = %q", got)
	}
	if got := d.BiomeAsset(); got != "biome-linux-x64" {
		t.Errorf("fallback BiomeAsset = %q", got)
	}
	if got := d.RuffTarget(); got != "x86_64-unknown-linux-gnu" {
		t.Errorf("fallback RuffTarget = %q", got)
	}
	if _, _, _, err := (fallbackPhpSource{}).resolve("8.4"); err == nil {
		t.Error("fallbackPhpSource must return an error")
	}
}
