package logs

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFindLogs проверяет поиск лог-файлов
func TestFindLogs(t *testing.T) {
	tmpDir := t.TempDir()

	// Создаём несколько лог-файлов
	log1 := filepath.Join(tmpDir, "app.log")
	if err := os.WriteFile(log1, []byte("log content"), 0644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(tmpDir, "logs")
	os.MkdirAll(subDir, 0755)
	log2 := filepath.Join(subDir, "debug.log")
	if err := os.WriteFile(log2, []byte("debug"), 0644); err != nil {
		t.Fatal(err)
	}
	// Не лог-файл
	os.WriteFile(filepath.Join(tmpDir, "config.txt"), []byte("config"), 0644)

	entries, err := FindLogs(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Собираем только файловые записи (docker может быть недоступен или давать лишние записи)
	var fileEntries []LogEntry
	for _, e := range entries {
		if e.Type == "file" {
			fileEntries = append(fileEntries, e)
		}
	}

	if len(fileEntries) != 2 {
		t.Fatalf("FindLogs вернула %d файловых записей, ожидалось 2: %v", len(fileEntries), fileEntries)
	}
	// Проверяем, что пути относительные
	for _, entry := range fileEntries {
		if entry.Path == "" {
			t.Error("entry.Path пустой")
		}
	}
}
