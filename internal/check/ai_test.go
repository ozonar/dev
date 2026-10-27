package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dev/internal/ai"
)

// TestReadScopeFiles_SkipsLargeFiles проверяет, что файлы, размер которых
// превышает общий лимит MaxReviewFileSize, исключаются из ревью целиком,
// а допустимые файлы попадают в текст.
func TestReadScopeFiles_SkipsLargeFiles(t *testing.T) {
	tmp := t.TempDir()

	small := filepath.Join(tmp, "small.go")
	if err := os.WriteFile(small, []byte("package small\n"), 0o644); err != nil {
		t.Fatalf("write small: %v", err)
	}

	large := filepath.Join(tmp, "large.go")
	big := make([]byte, ai.MaxReviewFileSize+1)
	for i := range big {
		big[i] = 'a'
	}
	if err := os.WriteFile(large, big, 0o644); err != nil {
		t.Fatalf("write large: %v", err)
	}

	text := readScopeFiles([]string{small, large})

	if !strings.Contains(text, "small.go") || !strings.Contains(text, "package small") {
		t.Errorf("small file should be included in review text, got: %q", text)
	}
	if strings.Contains(text, "large.go") {
		t.Errorf("large file should be excluded from review text")
	}
}

// TestReadScopeFiles_BoundarySize проверяет, что файл размером ровно
// MaxReviewFileSize включается в ревью (лимит — строгое превышение).
func TestReadScopeFiles_BoundarySize(t *testing.T) {
	tmp := t.TempDir()

	exact := filepath.Join(tmp, "exact.go")
	data := make([]byte, ai.MaxReviewFileSize)
	for i := range data {
		data[i] = 'b'
	}
	if err := os.WriteFile(exact, data, 0o644); err != nil {
		t.Fatalf("write exact: %v", err)
	}

	text := readScopeFiles([]string{exact})
	if !strings.Contains(text, "exact.go") {
		t.Errorf("file of exact limit size should be included in review text")
	}
}

// TestReadScopeFiles_MissingFile проверяет, что несуществующий файл
// молча пропускается.
func TestReadScopeFiles_MissingFile(t *testing.T) {
	text := readScopeFiles([]string{filepath.Join(t.TempDir(), "missing.go")})
	if text != "" {
		t.Errorf("missing file should be skipped, got: %q", text)
	}
}

// TestReadScopeFiles_SkipsBinary проверяет, что бинарный файл (содержащий
// NUL-байт) исключается из текста ревью, а текстовый файл рядом остаётся.
func TestReadScopeFiles_SkipsBinary(t *testing.T) {
	tmp := t.TempDir()

	textFile := filepath.Join(tmp, "main.go")
	if err := os.WriteFile(textFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write text file: %v", err)
	}

	// Сигнатура PNG содержит NUL-байты — это признак бинарного файла.
	binaryFile := filepath.Join(tmp, "icon.png")
	if err := os.WriteFile(binaryFile, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00}, 0o644); err != nil {
		t.Fatalf("write binary file: %v", err)
	}

	text := readScopeFiles([]string{textFile, binaryFile})

	if !strings.Contains(text, "main.go") {
		t.Errorf("text file should be included in review text, got: %q", text)
	}
	if strings.Contains(text, "icon.png") {
		t.Errorf("binary file should be skipped from review text, got: %q", text)
	}
}
