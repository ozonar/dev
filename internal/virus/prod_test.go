//go:build !windows

package virus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCopyProdCommandFromMissingDir проверяет, что при отсутствии локальной
// папки конфигов копирование пропускается без ошибки.
func TestCopyProdCommandFromMissingDir(t *testing.T) {
	// Несуществующий путь к папке конфигов
	missingDir := filepath.Join(t.TempDir(), "нет-такой-папки")

	err := copyProdCommandFrom(missingDir, "user", "127.0.0.1")
	if err != nil {
		t.Errorf("ожидался nil при отсутствии папки, получена ошибка: %v", err)
	}
}

// TestCopyProdCommandFromNotDir проверяет, что когда по пути лежит файл,
// а не папка, возвращается ошибка.
func TestCopyProdCommandFromNotDir(t *testing.T) {
	// Создаём файл вместо папки
	filePath := filepath.Join(t.TempDir(), "prod-command")
	if err := os.WriteFile(filePath, []byte("не папка"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := copyProdCommandFrom(filePath, "user", "127.0.0.1")
	if err == nil {
		t.Error("ожидалась ошибка, когда путь не является папкой")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("сообщение об ошибке должно содержать 'is not a directory', получено: %v", err)
	}
}

// TestStageWithoutReports проверяет, что при подготовке копии каталога
// конфигов подпапка reports с историей отчётов исключается.
func TestStageWithoutReports(t *testing.T) {
	src := t.TempDir()
	// Файл конфига, который должен попасть в копию.
	if err := os.WriteFile(filepath.Join(src, "deps.conf"), []byte("dep=https://example.com"), 0o644); err != nil {
		t.Fatal(err)
	}
	// История отчётов, которая должна быть исключена.
	reportsDir := filepath.Join(src, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reportsDir, "2026-01-01_00-00-00.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	staged, err := stageWithoutReports(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(staged))

	// Имя итоговой папки совпадает с именем исходного каталога.
	if filepath.Base(staged) != filepath.Base(src) {
		t.Errorf("ожидалось имя %s, получено %s", filepath.Base(src), filepath.Base(staged))
	}
	// Конфиг перенесён, история отчётов — нет.
	if _, err := os.Stat(filepath.Join(staged, "deps.conf")); err != nil {
		t.Errorf("deps.conf должен быть в копии: %v", err)
	}
	if _, err := os.Stat(filepath.Join(staged, "reports")); !os.IsNotExist(err) {
		t.Errorf("reports не должен попадать в копию, получена ошибка: %v", err)
	}
}

// TestResolveFirstExisting проверяет поиск первого существующего файла
// из списка путей в порядке приоритета.
func TestResolveFirstExisting(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.conf")
	second := filepath.Join(dir, "second.conf")
	if err := os.WriteFile(second, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Первый путь не существует — должен быть выбран второй.
	if got := resolveFirstExisting([]string{first, second}); got != second {
		t.Errorf("ожидался %s, получен %q", second, got)
	}

	// Ни одного существующего — пустая строка.
	if got := resolveFirstExisting([]string{first}); got != "" {
		t.Errorf("ожидалась пустая строка, получена %q", got)
	}

	// Директория не считается файлом конфига.
	if got := resolveFirstExisting([]string{dir}); got != "" {
		t.Errorf("директория не должна считаться конфигом, получена %q", got)
	}
}
