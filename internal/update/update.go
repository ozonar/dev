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

	"github.com/fatih/color"
)

const (
	repoURL = "https://github.com/ozonar/dev/releases/latest/download"
)

// SelfUpdate скачивает последнюю версию dev, устанавливает её и удаляет временный файл.
func SelfUpdate() error {
	// Определяем архитектуру и ОС
	arch := runtime.GOARCH
	goos := runtime.GOOS

	// Маппинг архитектур к именам в релизах
	archMap := map[string]string{
		"amd64": "amd64",
		"arm64": "aarch64",
	}
	releaseArch, ok := archMap[arch]
	if !ok {
		return fmt.Errorf("unsupported architecture: %s", arch)
	}

	// Имя файла в релизе: dev-{os}-{arch}, для windows добавляем .exe
	releaseFile := fmt.Sprintf("dev-%s-%s", goos, releaseArch)
	if goos == "windows" {
		releaseFile += ".exe"
	}
	downloadURL := fmt.Sprintf("%s/%s", repoURL, releaseFile)

	// Определяем домашнюю директорию
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get home directory: %v", err)
	}

	// Скачиваем под именем "dev" (или "dev.exe" на windows)
	tmpName := "dev"
	if goos == "windows" {
		tmpName = "dev.exe"
	}
	tmpPath := filepath.Join(home, tmpName)

	color.Cyan("Downloading %s ...", downloadURL)

	// Скачиваем файл
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	outFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("could not create file %s: %v", tmpPath, err)
	}

	written, err := io.Copy(outFile, resp.Body)
	if err != nil {
		outFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("file write failed: %v", err)
	}
	outFile.Close()

	// Устанавливаем права на выполнение
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("could not set executable permissions: %v", err)
	}

	color.Green("Downloaded %d bytes to %s", written, tmpPath)

	// Определяем текущий путь к dev через which/where
	currentPath, err := findDevPath()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("could not determine current dev path: %v", err)
	}

	color.Cyan("Current dev path: %s", currentPath)
	color.Cyan("Installing new version from the downloaded file...")

	// Запускаем скачанный файл с командой install
	// Передаём текущий путь как аргумент, чтобы install знал куда копировать
	cmd := exec.Command(tmpPath, "install")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("installation failed: %v", err)
	}

	// Удаляем скачанный файл
	color.Cyan("Removing temporary file...")
	if err := os.Remove(tmpPath); err != nil {
		return fmt.Errorf("could not remove temporary file %s: %v", tmpPath, err)
	}

	color.Green("Update completed successfully!")
	return nil
}

// findDevPath находит путь к текущему исполняемому файлу dev через which/where.
func findDevPath() (string, error) {
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
		cmd = exec.Command("where", "dev")
	} else {
		cmd = exec.Command("which", "dev")
	}

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("dev not found in PATH")
	}

	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("dev not found in PATH")
	}

	return path, nil
}
