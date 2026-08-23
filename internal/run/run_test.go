package run

import (
	"dev/internal/common"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withFakeBinary добавляет во временную директорию фейковый исполняемый
// файл name и помещает эту директорию в начало PATH. Позволяет замокать
// резолюцию рантайма (ResolveRuntime найдёт фейковый бинарь без обращения
// к сети) и тестировать логику RunProject без сетевых запросов.
//
// Скрипт выводит версию 8.3, чтобы systemProgram мог определить системный
// рантайм (для php версия обязательна; для npm вывод не используется).
func withFakeBinary(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"8.3\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })
}

// TestFindGoMainRun проверяет поиск main файлов (аналогично build, но своя реализация)
func TestFindGoMainRun(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Создаём main.go в корне
	mainRoot := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainRoot, []byte("package main\nfunc main(){}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Создаём ещё один main в поддиректории
	subDir := filepath.Join(tmpDir, "cmd", "app")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	mainSub := filepath.Join(subDir, "main.go")
	if err := os.WriteFile(mainSub, []byte("package main\nfunc main(){}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Не-main файл
	if err := os.WriteFile(filepath.Join(tmpDir, "utils.go"), []byte("package utils"), 0644); err != nil {
		t.Fatal(err)
	}

	mains, err := common.FindGoMain(".", common.FindGoMainOptions{
		SearchInCmdFirst: false,
		ExcludeDirs:      []string{},
		OnlyMainGo:       false,
	})
	if err != nil {
		t.Fatalf("FindGoMain вернула ошибку: %v", err)
	}
	if len(mains) != 2 {
		t.Fatalf("FindGoMain вернула %d файлов, ожидалось 2: %v", len(mains), mains)
	}
	// Проверяем, что оба файлы найдены
	foundRoot := false
	foundSub := false
	for _, m := range mains {
		if strings.HasSuffix(m, "main.go") {
			if strings.Contains(m, "cmd/app") {
				foundSub = true
			} else if strings.Contains(m, "main.go") && !strings.Contains(m, "cmd") {
				foundRoot = true
			}
		}
	}
	if !foundRoot || !foundSub {
		t.Errorf("не все main файлы найдены: foundRoot=%v, foundSub=%v", foundRoot, foundSub)
	}
}

// TestRunProjectUnsupported проверяет ошибку для неподдерживаемого фреймворка
func TestRunProjectUnsupported(t *testing.T) {
	err := RunProject("unknown", "")
	if err == nil {
		t.Error("ожидалась ошибка для неподдерживаемого фреймворка")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("сообщение об ошибке должно содержать 'unsupported', получили: %v", err)
	}
}

// TestRunProjectLaravelNoArtisan проверяет ошибку при отсутствии artisan
func TestRunProjectLaravelNoArtisan(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Мокаем php, чтобы резолюция рантайма не уходила в сеть.
	withFakeBinary(t, "php")

	err = RunProject("laravel", "php")
	if err == nil {
		t.Error("ожидалась ошибка 'artisan not found'")
	}
	if !strings.Contains(err.Error(), "artisan") {
		t.Errorf("сообщение об ошибке должно содержать 'artisan', получили: %v", err)
	}
}

// TestRunProjectNodeNoPackageJson аналогично
func TestRunProjectNodeNoPackageJson(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Мокаем npm, чтобы резолюция рантайма не уходила в сеть.
	withFakeBinary(t, "npm")

	err = RunProject("node", "javascript")
	if err == nil {
		t.Error("ожидалась ошибка 'package.json not found'")
	}
	if !strings.Contains(err.Error(), "package.json") {
		t.Errorf("сообщение об ошибке должно содержать 'package.json', получили: %v", err)
	}
}

// TestIsPortInUseError проверяет детекцию ошибок занятого порта
func TestIsPortInUseError(t *testing.T) {
	tests := []struct {
		errStr string
		want   bool
	}{
		{"address already in use", true},
		{"Address already in use", true},
		{"bind: address already in use", true},
		{"port already in use", true},
		{"only one usage of each socket address", true},
		{"EADDRINUSE", true},
		{"addr already in use", true},
		{"connection refused", false},
		{"no such file or directory", false},
		{"permission denied", false},
		{"", false},
		{"some random error", false},
	}
	for _, tt := range tests {
		got := isPortInUseError(tt.errStr)
		if got != tt.want {
			t.Errorf("isPortInUseError(%q) = %v, want %v", tt.errStr, got, tt.want)
		}
	}
}

// TestRunProjectWithOptionsUnsupported проверяет RunProjectWithOptions для неподдерживаемого фреймворка
func TestRunProjectWithOptionsUnsupported(t *testing.T) {
	err := RunProjectWithOptions("unknown", "", RunOptions{Port: 8080})
	if err == nil {
		t.Error("ожидалась ошибка для неподдерживаемого фреймворка")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("сообщение об ошибке должно содержать 'unsupported', получили: %v", err)
	}
}

// TestRunProjectWithOptionsPort проверяет, что опция порта передаётся без паники
func TestRunProjectWithOptionsPort(t *testing.T) {
	// Проверяем, что для unsupported фреймворка порт игнорируется
	err := RunProjectWithOptions("unknown", "", RunOptions{Port: 9999})
	if err == nil {
		t.Error("ожидалась ошибка для неподдерживаемого фреймворка")
	}
}

// TestRunProjectGoNoMain проверяет ошибку при отсутствии main файлов
func TestRunProjectGoNoMain(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	err = RunProject("go", "go")
	if err == nil {
		t.Error("ожидалась ошибка 'no Go main files found'")
	}
	if !strings.Contains(err.Error(), "no Go main") {
		t.Errorf("сообщение об ошибке должно содержать 'no Go main', получили: %v", err)
	}
}

// TestIsBinaryAvailable проверяет определение доступности бинарника в PATH
func TestIsBinaryAvailable(t *testing.T) {
	// Пустое имя — всегда false
	if isBinaryAvailable("") {
		t.Error("isBinaryAvailable(\"\") должно возвращать false")
	}

	// Несуществующий бинарник — false
	if isBinaryAvailable("__definitely_not_a_real_binary_12345__") {
		t.Error("isBinaryAvailable для несуществующего бинарника должно возвращать false")
	}

	// Известные системные бинарники должны быть доступны (например, sh)
	if !isBinaryAvailable("sh") {
		t.Error("isBinaryAvailable(\"sh\") должно возвращать true")
	}
}
