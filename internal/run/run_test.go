package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFindGoMainRun проверяет поиск main файлов (аналогично build, но своя реализация)
func TestFindGoMainRun(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Создаём main.go в корне
	mainRoot := filepath.Join(tmpDir, "main.go")
	os.WriteFile(mainRoot, []byte("package main\nfunc main(){}"), 0644)

	// Создаём ещё один main в поддиректории
	subDir := filepath.Join(tmpDir, "cmd", "app")
	os.MkdirAll(subDir, 0755)
	mainSub := filepath.Join(subDir, "main.go")
	os.WriteFile(mainSub, []byte("package main\nfunc main(){}"), 0644)

	// Не-main файл
	os.WriteFile(filepath.Join(tmpDir, "utils.go"), []byte("package utils"), 0644)

	mains := findGoMain()
	if len(mains) != 2 {
		t.Fatalf("findGoMain вернула %d файлов, ожидалось 2: %v", len(mains), mains)
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
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

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
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	err = RunProject("node", "javascript")
	if err == nil {
		t.Error("ожидалась ошибка 'package.json not found'")
	}
	if !strings.Contains(err.Error(), "package.json") {
		t.Errorf("сообщение об ошибке должно содержать 'package.json', получили: %v", err)
	}
}

// TestRunProjectGoNoMain проверяет ошибку при отсутствии main файлов
func TestRunProjectGoNoMain(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
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
