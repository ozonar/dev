package virus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScpDirMissingDir проверяет, что при отсутствии локальной папки функция
// не возвращает ошибку (копирование пропускается).
func TestScpDirMissingDir(t *testing.T) {
	// Несуществующий путь к папке конфигов
	missingDir := filepath.Join(t.TempDir(), "не-существующая-папка")

	err := scpDir(missingDir, "user", "127.0.0.1", "/home/user/dev-config")
	if err != nil {
		t.Errorf("ожидался nil при отсутствии папки конфигов, получена ошибка: %v", err)
	}
}

// TestScpDirNotDir проверяет, что когда по пути лежит файл, а не папка,
// возвращается ошибка.
func TestScpDirNotDir(t *testing.T) {
	// Создаём файл вместо папки
	filePath := filepath.Join(t.TempDir(), "dev-config")
	if err := os.WriteFile(filePath, []byte("не папка"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := scpDir(filePath, "user", "127.0.0.1", "/home/user/dev-config")
	if err == nil {
		t.Error("ожидалась ошибка, когда путь не является папкой")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("сообщение об ошибке должно содержать 'is not a directory', получено: %v", err)
	}
}

// TestParseTargetUserHost проверяет разбор строки подключения "user@host".
func TestParseTargetUserHost(t *testing.T) {
	username, host, err := parseTarget("root@192.168.1.10")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if username != "root" || host != "192.168.1.10" {
		t.Errorf("ожидались root/192.168.1.10, получены %s/%s", username, host)
	}
}

// TestParseTargetInvalid проверяет, что строка с несколькими '@' отвергается.
func TestParseTargetInvalid(t *testing.T) {
	if _, _, err := parseTarget("a@b@c"); err == nil {
		t.Error("ожидалась ошибка для некорректного формата")
	}
}

// TestHomeDirFor проверяет выбор домашнего каталога для root и обычного
// пользователя.
func TestHomeDirFor(t *testing.T) {
	if got := homeDirFor("root"); got != "/root" {
		t.Errorf("ожидался /root, получен %s", got)
	}
	if got := homeDirFor("deploy"); got != "/home/deploy" {
		t.Errorf("ожидался /home/deploy, получен %s", got)
	}
}
