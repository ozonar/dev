package update

import "testing"

// TestReleaseFileName проверяет, что имя файла релиза совпадает с артефактами
// CI (build.yml и prod.yml): {name}-{goos}-{goarch}, для Windows — с .exe.
func TestReleaseFileName(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		goarch string
		want   string
	}{
		{"dev", "linux", "amd64", "dev-linux-amd64"},
		{"dev", "windows", "amd64", "dev-windows-amd64.exe"},
		{"prod", "linux", "amd64", "prod-linux-amd64"},
	}
	for _, tt := range tests {
		got := releaseFileName(tt.name, tt.goos, tt.goarch)
		if got != tt.want {
			t.Errorf("releaseFileName(%q, %q, %q) = %q, want %q",
				tt.name, tt.goos, tt.goarch, got, tt.want)
		}
	}
}
