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
// Скачивание идёт в файл с уникальным именем, затем файл атомарно переименовывается
// в $HOME/{name}, поэтому обновление работает, даже если бинарник {name} в данный
// момент запущен (прямая перезапись исполняемого файла на Linux даёт ETXTBSY).
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

	// Путь, по которому будет размещён скачанный бинарник. Имя файла должно
	// совпадать с именем устанавливаемого бинарника, иначе команда install
	// запишет его под неправильным именем.
	finalPath := finalBinaryPath(home, name, goos)

	i18n.Cyan("Downloading %s ...", downloadURL)

	// Скачиваем во временный файл с уникальным именем. Писать сразу в
	// finalPath нельзя: если там находится запущенный бинарник, Linux вернёт
	// ETXTBSY (text file busy) — исполняемый файл работающего процесса
	// запрещено перезаписывать. CreateTemp создаёт новый файл, не трогая
	// существующий.
	tmpFile, err := os.CreateTemp(home, name+".tmp-*")
	if err != nil {
		return fmt.Errorf(i18n.T("could not create temporary file in %s: %v"), home, err)
	}
	tmpPath := tmpFile.Name()

	// Скачиваем файл
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf(i18n.T("download failed: %v"), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf(i18n.T("server returned status %d"), resp.StatusCode)
	}

	written, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf(i18n.T("file write failed: %v"), err)
	}
	tmpFile.Close()

	// Устанавливаем права на выполнение
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf(i18n.T("could not set executable permissions: %v"), err)
	}

	i18n.Green("Downloaded %d bytes to %s", written, tmpPath)

	// Атомарно перемещаем временный файл в finalPath. rename() не открывает
	// целевой файл на запись, поэтому работает, даже если finalPath — это
	// запущенный в данный момент бинарник: старая версия продолжит
	// выполняться из своего inode, а по пути окажется новая.
	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf(i18n.T("could not move downloaded file to %s: %v"), finalPath, err)
	}

	// Определяем текущий путь к бинарнику через which/where
	currentPath, err := findBinaryPath(name)
	if err != nil {
		os.Remove(finalPath)
		return fmt.Errorf(i18n.T("could not determine current %s path: %v"), name, err)
	}

	i18n.Cyan("Current %s path: %s", name, currentPath)
	i18n.Cyan("Installing new version from the downloaded file...")

	// Запускаем скачанный файл с командой install: install сам определяет
	// исходный файл как os.Executable() (скачанный бинарник) и спрашивает
	// директорию назначения.
	cmd := exec.Command(finalPath, "install")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		os.Remove(finalPath)
		return fmt.Errorf(i18n.T("installation failed: %v"), err)
	}

	// Удаляем скачанный файл
	i18n.Cyan("Removing temporary file...")
	if err := os.Remove(finalPath); err != nil {
		return fmt.Errorf(i18n.T("could not remove temporary file %s: %v"), finalPath, err)
	}

	i18n.Green("Update completed successfully!")
	return nil
}

// finalBinaryPath возвращает путь, по которому будет размещён скачанный
// бинарник: $HOME/{name} (на Windows — {name}.exe). Имя файла должно
// совпадать с именем устанавливаемого бинарника, иначе команда install
// запишет его под неправильным именем.
func finalBinaryPath(home, name, goos string) string {
	fileName := name
	if goos == "windows" {
		fileName += ".exe"
	}
	return filepath.Join(home, fileName)
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
