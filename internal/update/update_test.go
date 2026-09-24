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

// TestFinalBinaryPath проверяет путь размещения скачанного бинарника:
// $HOME/{name}, на Windows — $HOME/{name}.exe. Имя файла должно совпадать
// с именем устанавливаемого бинарника, иначе install запишет его неправильно.
func TestFinalBinaryPath(t *testing.T) {
	tests := []struct {
		home string
		name string
		goos string
		want string
	}{
		{"/root", "prod", "linux", "/root/prod"},
		{"/root", "dev", "linux", "/root/dev"},
		{"/home/user", "dev", "darwin", "/home/user/dev"},
		{"/home/user", "prod", "windows", "/home/user/prod.exe"},
	}
	for _, tt := range tests {
		got := finalBinaryPath(tt.home, tt.name, tt.goos)
		if got != tt.want {
			t.Errorf("finalBinaryPath(%q, %q, %q) = %q, want %q",
				tt.home, tt.name, tt.goos, got, tt.want)
		}
	}
}
