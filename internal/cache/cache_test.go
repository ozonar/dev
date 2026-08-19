package cache

import (
	"os"
	"path/filepath"
	"testing"
)

// TestClearCacheGeneric проверяет очистку generic кэша (директория cache)
func TestClearCacheGeneric(t *testing.T) {
	tmpDir := t.TempDir()
	// Меняем рабочую директорию на временную
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

	// Создаём директорию cache с файлами
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	file1 := filepath.Join(cacheDir, "file1.txt")
	if err := os.WriteFile(file1, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(cacheDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	file2 := filepath.Join(subDir, "file2.txt")
	if err := os.WriteFile(file2, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Вызываем очистку для generic
	err = ClearCache("generic")
	if err != nil {
		t.Fatal(err)
	}

	// Проверяем, что директория cache пуста
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("директория cache должна быть пустой, но содержит %d элементов", len(entries))
	}
}

// TestClearCachePython проверяет удаление __pycache__ и .pyc файлов
func TestClearCachePython(t *testing.T) {
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

	// Создаём __pycache__ директорию
	pycache := filepath.Join(tmpDir, "__pycache__")
	if err := os.MkdirAll(pycache, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pycache, "module.cpython-39.pyc"), []byte("bytecode"), 0644); err != nil {
		t.Fatal(err)
	}
	// Создаём .pyc файл
	pycFile := filepath.Join(tmpDir, "module.pyc")
	os.WriteFile(pycFile, []byte("bytecode"), 0644)

	err = ClearCache("python")
	if err != nil {
		t.Fatal(err)
	}

	// Проверяем, что __pycache__ удалена
	if _, err := os.Stat(pycache); !os.IsNotExist(err) {
		t.Error("__pycache__ не была удалена")
	}
	// Проверяем, что .pyc файл удалён
	if _, err := os.Stat(pycFile); !os.IsNotExist(err) {
		t.Error(".pyc файл не был удалён")
	}
}

// TestClearCacheUnsupported проверяет ошибку для неподдерживаемого фреймворка
func TestClearCacheUnsupported(t *testing.T) {
	err := ClearCache("unknown")
	if err == nil {
		t.Error("ожидалась ошибка для неподдерживаемого фреймворка")
	}
}

// TestClearCacheSymfonyNoArtisan проверяет, что нет ошибки если bin/console отсутствует
func TestClearCacheSymfonyNoArtisan(t *testing.T) {
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

	// Нет bin/console
	err = ClearCache("symfony")
	if err != nil {
		t.Errorf("ClearCache для symfony без bin/console должна вернуть nil, получили %v", err)
	}
}

// TestClearCacheLaravelNoArtisan аналогично
func TestClearCacheLaravelNoArtisan(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	err = ClearCache("laravel")
	if err != nil {
		t.Errorf("ClearCache для laravel без artisan должна вернуть nil, получили %v", err)
	}
}
