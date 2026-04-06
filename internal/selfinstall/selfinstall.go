package selfinstall

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// SelfInstall копирует текущий исполняемый файл в ~/bin (или /usr/local/bin) и устанавливает права на выполнение.
func SelfInstall() error {
	// Определяем путь к текущему исполняемому файлу
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить путь к исполняемому файлу: %v", err)
	}

	// Определяем целевую директорию
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("не удалось определить домашнюю директорию: %v", err)
	}
	targetDir := filepath.Join(home, "bin")

	// Создаём целевую директорию, если её нет
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию %s: %v", targetDir, err)
	}

	// Имя файла в целевой директории (можно оставить "dev" или взять базовое имя)
	targetPath := filepath.Join(targetDir, "dev")

	// Копируем файл
	srcFile, err := os.Open(exe)
	if err != nil {
		return fmt.Errorf("не удалось открыть исходный файл %s: %v", exe, err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("не удалось создать целевой файл %s: %v", targetPath, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("ошибка копирования: %v", err)
	}

	// Устанавливаем права на выполнение (chmod +x)
	if err := os.Chmod(targetPath, 0755); err != nil {
		return fmt.Errorf("не удалось установить права на выполнение: %v", err)
	}

	// Проверяем, что файл скопирован корректно
	if runtime.GOOS != "windows" {
		cmd := exec.Command(targetPath, "--help")
		if err := cmd.Run(); err != nil {
			// Игнорируем ошибку, так как --help может не поддерживаться
			fmt.Printf("Примечание: команда --help завершилась с ошибкой (возможно, нормально)\n")
		}
	}

	fmt.Printf("Успешно установлено в %s\n", targetPath)
	fmt.Printf("Убедитесь, что %s находится в PATH\n", targetDir)
	return nil
}
