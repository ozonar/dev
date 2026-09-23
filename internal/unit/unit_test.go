package unit

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeFile создаёт файл в тестовой директории и возвращает ошибку.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestBuildGoArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"no args uses all packages", nil, []string{"test", "./..."}},
		{"passes explicit package", []string{"./internal/..."}, []string{"test", "./internal/..."}},
		{"passes flags and paths", []string{"-race", "./..."}, []string{"test", "-race", "./..."}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildGoArgs(tc.args)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("buildGoArgs(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestDetectPackageManager(t *testing.T) {
	cases := []struct {
		name  string
		files []string // пары имя-содержимое, создаются в temp dir
		want  string
	}{
		{"empty project", nil, ""},
		{"npm by lock", []string{"package-lock.json:{}"}, "npm"},
		{"npm by package.json", []string{"package.json:{}"}, "npm"},
		{"yarn wins over npm", []string{"package-lock.json:{}", "yarn.lock:"}, "yarn"},
		{"pnpm wins over yarn", []string{"yarn.lock:", "pnpm-lock.yaml:"}, "pnpm"},
		{"pnpm workspace", []string{"pnpm-workspace.yaml:"}, "pnpm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				name, content := splitNameContent(f)
				writeFile(t, dir, name, content)
			}
			if got := detectPackageManager(dir); got != tc.want {
				t.Fatalf("detectPackageManager() = %q, want %q", got, tc.want)
			}
		})
	}
}

// splitNameContent разделяет запись "name:content" на имя файла и содержимое.
func splitNameContent(entry string) (string, string) {
	for i := 0; i < len(entry); i++ {
		if entry[i] == ':' {
			return entry[:i], entry[i+1:]
		}
	}
	return entry, ""
}

func TestHasNpmTestScript(t *testing.T) {
	dir := t.TempDir()

	// Без package.json — false.
	if hasNpmTestScript(dir) {
		t.Error("hasNpmTestScript() = true without package.json")
	}

	writeFile(t, dir, "package.json", `{"scripts": {"test": "jest"}}`)
	if !hasNpmTestScript(dir) {
		t.Error("hasNpmTestScript() = false with test script")
	}

	writeFile(t, dir, "package.json", `{"scripts": {"build": "tsc"}}`)
	if hasNpmTestScript(dir) {
		t.Error("hasNpmTestScript() = true without test script")
	}
}

func TestHasComposerTestScript(t *testing.T) {
	dir := t.TempDir()

	if hasComposerTestScript(dir) {
		t.Error("hasComposerTestScript() = true without composer.json")
	}

	// Скрипт может быть строкой или массивом — оба варианта должны распознаваться.
	writeFile(t, dir, "composer.json", `{"scripts": {"test": "phpunit"}}`)
	if !hasComposerTestScript(dir) {
		t.Error("hasComposerTestScript() = false with string test script")
	}

	writeFile(t, dir, "composer.json", `{"scripts": {"test": ["phpunit", "--colors"]}}`)
	if !hasComposerTestScript(dir) {
		t.Error("hasComposerTestScript() = false with array test script")
	}

	writeFile(t, dir, "composer.json", `{"scripts": {"lint": "php-cs-fixer"}}`)
	if hasComposerTestScript(dir) {
		t.Error("hasComposerTestScript() = true without test script")
	}
}

func TestHasPytestConfig(t *testing.T) {
	cases := []struct {
		name    string
		content string // содержимое файла (имя задаётся отдельно)
		file    string
		want    bool
	}{
		{name: "pytest.ini", file: "pytest.ini", content: "[pytest]\n", want: true},
		{name: "tox.ini with pytest section", file: "tox.ini", content: "[pytest]\ntestpaths=tests\n", want: true},
		{name: "pyproject tool section", file: "pyproject.toml", content: "[tool.pytest.ini_options]\n", want: true},
		{name: "setup.cfg tool section", file: "setup.cfg", content: "[tool:pytest]\n", want: true},
		{name: "no config", file: "", content: "", want: false},
		{name: "unrelated pyproject", file: "pyproject.toml", content: "[tool.black]\n", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.file != "" {
				writeFile(t, dir, tc.file, tc.content)
			}
			if got := hasPytestConfig(dir); got != tc.want {
				t.Fatalf("hasPytestConfig() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFileAndDirExists(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "data")

	if !fileExists(filepath.Join(dir, "file.txt")) {
		t.Error("fileExists() = false for existing file")
	}
	if fileExists(filepath.Join(dir, "missing.txt")) {
		t.Error("fileExists() = true for missing file")
	}
	// Директория не считается файлом.
	if fileExists(dir) {
		t.Error("fileExists() = true for directory")
	}
	if !dirExists(dir) {
		t.Error("dirExists() = false for existing directory")
	}
	if dirExists(filepath.Join(dir, "missing")) {
		t.Error("dirExists() = true for missing directory")
	}
}
