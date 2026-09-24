package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"dev/internal/i18n"
)

const (
	repoURL = "https://github.com/ozonar/dev/releases/latest/download"
)

// releaseFileName формирует имя файла релиза вида {name}-{goos}-{goarch}.
// Имя совпадает с артефактами CI: dev-linux-amd64, prod-linux-arm64 и т.д.
// Для Windows добавляется расширение .exe.
func releaseFileName(name, goos, goarch string) string {
	fileName := fmt.Sprintf("%s-%s-%s", name, goos, goarch)
	if goos == "windows" {
		fileName += ".exe"
	}
	return fileName
}

// SelfUpdate скачивает последнюю версию указанного бинарника (dev или prod),
// устанавливает её через подкоманду install скачанного файла и удаляет временный файл.
func SelfUpdate(name string) error {
	// Определяем архитектуру и ОС
	goarch := runtime.GOARCH
	goos := runtime.GOOS

	// Имя файла в релизе: {name}-{os}-{arch}, для windows добавляем .exe
	releaseFile := releaseFileName(name, goos, goarch)
	downloadURL := fmt.Sprintf("%s/%s", repoURL, releaseFile)

	// Определяем домашнюю директорию
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf(i18n.T("could not get home directory: %v"), err)
	}

	// Скачиваем под именем {name} (или {name}.exe на windows)
	tmpName := name
	if goos == "windows" {
		tmpName += ".exe"
	}
	tmpPath := filepath.Join(home, tmpName)

	i18n.Cyan("Downloading %s ...", downloadURL)

	// Скачиваем файл
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf(i18n.T("download failed: %v"), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(i18n.T("server returned status %d"), resp.StatusCode)
	}

	outFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf(i18n.T("could not create file %s: %v"), tmpPath, err)
	}

	written, err := io.Copy(outFile, resp.Body)
	if err != nil {
		outFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf(i18n.T("file write failed: %v"), err)
	}
	outFile.Close()

	// Устанавливаем права на выполнение
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf(i18n.T("could not set executable permissions: %v"), err)
	}

	i18n.Green("Downloaded %d bytes to %s", written, tmpPath)

	// Определяем текущий путь к бинарнику через which/where
	currentPath, err := findBinaryPath(name)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf(i18n.T("could not determine current %s path: %v"), name, err)
	}

	i18n.Cyan("Current %s path: %s", name, currentPath)
	i18n.Cyan("Installing new version from the downloaded file...")

	// Запускаем скачанный файл с командой install: install сам определяет
	// исходный файл как os.Executable() (скачанный бинарник) и спрашивает
	// директорию назначения.
	cmd := exec.Command(tmpPath, "install")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf(i18n.T("installation failed: %v"), err)
	}

	// Удаляем скачанный файл
	i18n.Cyan("Removing temporary file...")
	if err := os.Remove(tmpPath); err != nil {
		return fmt.Errorf(i18n.T("could not remove temporary file %s: %v"), tmpPath, err)
	}

	i18n.Green("Update completed successfully!")
	return nil
}

// findBinaryPath находит путь к текущему исполняемому файлу {name} через which/where.
func findBinaryPath(name string) (string, error) {
	// Сначала пробуем os.Executable() — это путь к текущему процессу
	exe, err := os.Executable()
	if err == nil {
		// Проверяем, что файл существует
		if _, err := os.Stat(exe); err == nil {
			return exe, nil
		}
	}

	// Fallback: ищем через which (linux/mac) или where (windows)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("where", name)
	} else {
		cmd = exec.Command("which", name)
	}

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf(i18n.T("%s not found in PATH"), name)
	}

	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf(i18n.T("%s not found in PATH"), name)
	}

	return path, nil
}
