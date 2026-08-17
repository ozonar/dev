package virus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCopyDevConfigDirMissingDir проверяет, что при отсутствии локальной папки
// конфигов функция не возвращает ошибку (копирование пропускается).
func TestCopyDevConfigDirMissingDir(t *testing.T) {
	// Несуществующий путь к папке конфигов
	missingDir := filepath.Join(t.TempDir(), "не-существующая-папка")

	err := copyDevConfigDir(missingDir, "user", "127.0.0.1", "/home/user")
	if err != nil {
		t.Errorf("ожидался nil при отсутствии папки конфигов, получена ошибка: %v", err)
	}
}

// TestCopyDevConfigDirNotDir проверяет, что когда по пути лежит файл,
// а не папка, возвращается ошибка.
func TestCopyDevConfigDirNotDir(t *testing.T) {
	// Создаём файл вместо папки
	filePath := filepath.Join(t.TempDir(), "dev-config")
	if err := os.WriteFile(filePath, []byte("не папка"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := copyDevConfigDir(filePath, "user", "127.0.0.1", "/home/user")
	if err == nil {
		t.Error("ожидалась ошибка, когда путь не является папкой")
	}
	if !strings.Contains(err.Error(), "не является папкой") {
		t.Errorf("сообщение об ошибке должно содержать 'не является папкой', получено: %v", err)
	}
}
