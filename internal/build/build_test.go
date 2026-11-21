package build

import "testing"

// TestResolveOutput проверяет выбор имени выходного файла с учётом флага -o
func TestResolveOutput(t *testing.T) {
	tests := []struct {
		target string
		output string
		want   string
	}{
		{"cmd/foo/main.go", "", "foo"},
		{"main.go", "", "main"},
		{"cmd/foo/main.go", "mybin", "mybin"},
		{"main.go", "./out/app", "./out/app"},
		{"cmd/bar/main.go", "custom", "custom"},
	}
	for _, tt := range tests {
		got := resolveOutput(tt.target, tt.output)
		if got != tt.want {
			t.Errorf("resolveOutput(%q, %q) = %q, want %q", tt.target, tt.output, got, tt.want)
		}
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
		{"./cmd/prod", "prod"}, // директория пакета после нормализации
		{".", "main"},          // пакет в корне проекта
	}
	for _, tt := range tests {
		got := outputName(tt.target)
		if got != tt.want {
			t.Errorf("outputName(%q) = %q, want %q", tt.target, got, tt.want)
		}
	}
}
