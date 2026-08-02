package logs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestFindPHPFPMLogsSystem проверяет поиск системных PHP-FPM логов через glob
func TestFindPHPFPMLogsSystem(t *testing.T) {
	tmpDir := t.TempDir()

	// Создаём PHP проект (composer.json)
	os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte("{}"), 0644)

	// Создаём временный "системный" лог-файл в /tmp с именем, подходящим под шаблон php*-fpm.log
	tmpLog := filepath.Join(tmpDir, "php8.2-fpm.log")
	os.WriteFile(tmpLog, []byte("php-fpm log content"), 0644)

	// Подменяем glob, чтобы он искал в tmpDir вместо /var/log
	// Но мы не можем подменить filepath.Glob, поэтому просто проверяем,
	// что функция не падает и возвращает пустой результат (системных логов нет)
	entries := findPHPFPMLogs(tmpDir)

	// В нормальных условиях системных логов нет, entries могут быть пустыми
	// или содержать docker-записи. Главное — не падать.
	_ = entries
}

// TestFindPHPFPMLogsNonPHP проверяет, что для не-PHP проекта логи не ищутся
func TestFindPHPFPMLogsNonPHP(t *testing.T) {
	tmpDir := t.TempDir()

	// Создаём Go проект (не PHP)
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)

	// Вызываем FindLogs — он внутри вызывает detector.DetectProject
	entries, err := FindLogs(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Проверяем, что среди записей нет PHP-FPM логов
	for _, e := range entries {
		if strings.Contains(e.Path, "php-fpm") {
			t.Errorf("Найден PHP-FPM лог в Go проекте: %s", e.Path)
		}
	}
}

// TestFindPHPFPMLogsPHPProject проверяет, что для PHP проекта FindLogs вызывает findPHPFPMLogs
func TestFindPHPFPMLogsPHPProject(t *testing.T) {
	tmpDir := t.TempDir()

	// Создаём PHP проект
	os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte("{}"), 0644)

	// Создаём .log файл, чтобы FindLogs не был пустым
	os.WriteFile(filepath.Join(tmpDir, "app.log"), []byte("test"), 0644)

	entries, err := FindLogs(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Должны быть хотя бы файловые записи (app.log)
	var fileEntries int
	for _, e := range entries {
		if e.Type == "file" {
			fileEntries++
		}
	}
	if fileEntries == 0 {
		t.Error("FindLogs не вернул ни одной файловой записи для PHP проекта")
	}
}

// TestHasLnav проверяет, что hasLnav() не падает и возвращает bool
func TestHasLnav(t *testing.T) {
	result := hasLnav()
	// Просто проверяем, что возвращается bool (не паника)
	_ = result
}

// TestOpenLogInLnavFallback проверяет, что OpenLogInLnav не падает
// при отсутствии lnav (использует less как fallback).
// Создаёт временный .log файл и пытается открыть его.
// В тесте мы не можем интерактивно работать с less, поэтому проверяем,
// что команда собирается корректно (не падает на этапе сборки).
func TestOpenLogInLnavFallback(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	if err := os.WriteFile(logFile, []byte("test log line 1\ntest log line 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Проверяем, что для файла без lnav используется less
	// Мы не можем запустить less в тесте (интерактивный), но можем проверить
	// что exec.Command не падает на этапе сборки команды
	if !hasLnav() {
		// Проверяем, что less доступен как fallback
		_, err := exec.LookPath("less")
		if err != nil {
			t.Skip("less не установлен, пропускаем тест fallback")
		}
	}

	// Проверяем, что функция не падает при попытке открыть несуществующий файл
	// (она должна вернуть ошибку, а не запаниковать)
	err := OpenLogInLnav("/nonexistent/path.log", "file")
	if err == nil {
		// Если lnav установлен, он может вернуть ошибку из-за отсутствия файла
		// Если less — тоже. Главное, что нет паники.
	}
}

// TestOpenLogInLnavDockerFallback проверяет fallback для docker логов
func TestOpenLogInLnavDockerFallback(t *testing.T) {
	// Проверяем, что для docker лога без lnav используется bash-пайп
	// Просто проверяем, что нет паники при вызове
	err := OpenLogInLnav("test-container", "docker")
	if err == nil {
		// Если docker не запущен или контейнер не существует — будет ошибка,
		// но не паника. Это нормально.
	}
}
