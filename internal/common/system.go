package common

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExpandHome раскрывает "~" и префикс "~/" в домашнюю директорию пользователя.
// Пути, не начинающиеся с "~", возвращаются без изменений.
func ExpandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

// IsLocalHost сообщает, что хост указывает на локальную машину.
func IsLocalHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0":
		return true
	}
	return false
}

// EditorCommand возвращает команду редактора из переменной $EDITOR.
// Если переменная не задана, используется nano. Команда разбивается на
// поля, поэтому поддерживаются значения с флагами (например "code --wait").
func EditorCommand() []string {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}
	return strings.Fields(editor)
}

// OpenInEditor открывает файл в редакторе из $EDITOR (по умолчанию nano)
// с передачей стандартных потоков терминала.
func OpenInEditor(path string) error {
	parts := EditorCommand()
	args := append(append([]string{}, parts[1:]...), path)
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
