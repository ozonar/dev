package virus

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// parseTarget разбирает строку подключения "user@host" или просто "host".
// При отсутствии пользователя используется текущий системный пользователь
// (fallback: переменная окружения USER, затем root).
func parseTarget(path string) (username, host string, err error) {
	if strings.Contains(path, "@") {
		parts := strings.Split(path, "@")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid path format. Expected user@ip")
		}
		return parts[0], parts[1], nil
	}

	// Просто IP-адрес или hostname — используем текущего пользователя.
	host = path
	if current, err := user.Current(); err == nil {
		return current.Username, host, nil
	}
	username = os.Getenv("USER")
	if username == "" {
		username = "root"
	}
	return username, host, nil
}

// homeDirFor возвращает домашний каталог пользователя на удалённом сервере.
func homeDirFor(username string) string {
	if username == "root" {
		return "/root"
	}
	return filepath.Join("/home", username)
}

// copyBinary копирует исполняемый файл на удалённый сервер через SCP
// и устанавливает права на выполнение.
func copyBinary(exe, username, host, remotePath string) error {
	cmd := exec.Command("scp", "-o", "StrictHostKeyChecking=no",
		exe, fmt.Sprintf("%s@%s:%s", username, host, remotePath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("Copying %s to %s@%s:%s...\n", filepath.Base(exe), username, host, remotePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("SCP failed: %v", err)
	}

	// Устанавливаем права на выполнение на удалённом сервере.
	if err := sshRun(username, host, "chmod +x "+remotePath); err != nil {
		fmt.Printf("Warning: could not set executable permissions on remote server: %v\n", err)
	}
	return nil
}

// scpDir рекурсивно копирует локальную папку в указанное место на удалённом
// сервере. Отсутствующая локальная папка пропускается без ошибки.
func scpDir(localDir, username, host, remoteDest string) error {
	info, err := os.Stat(localDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Config directory %s not found, skipping config copy.\n", localDir)
			return nil
		}
		return fmt.Errorf("could not check config directory: %v", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", localDir)
	}

	fmt.Printf("Copying configs from %s to %s@%s:%s...\n", localDir, username, host, remoteDest)
	cmd := exec.Command("scp", "-r", "-o", "StrictHostKeyChecking=no",
		localDir, fmt.Sprintf("%s@%s:%s", username, host, remoteDest))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("config SCP failed: %v", err)
	}
	return nil
}

// sshRun выполняет команду на удалённом сервере через SSH (по ключам).
func sshRun(username, host, command string) error {
	cmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no",
		fmt.Sprintf("%s@%s", username, host), command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Virus копирует текущий исполняемый файл на удалённый сервер через SCP.
// Параметр path должен быть в формате "user@ip" или просто "ip".
// Используется аутентификация по SSH-ключам (пароль не поддерживается).
func Virus(path string) error {
	// Определяем путь к текущему исполняемому файлу.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %v", err)
	}

	// Парсим строку подключения.
	username, host, err := parseTarget(path)
	if err != nil {
		return err
	}

	remotePath := homeDirFor(username)

	// Копируем бинарник на удалённый сервер и ставим права.
	if err := copyBinary(exe, username, host, remotePath); err != nil {
		return err
	}

	// Копируем все конфиги из папки ~/dev-config на удалённый сервер.
	if err := copyDevConfig(username, host, remotePath); err != nil {
		fmt.Printf("Warning: could not copy dev-config files: %v\n", err)
	}

	fmt.Printf("Successfully copied to %s:%s\n", host, remotePath)
	return nil
}

// copyDevConfig копирует всё содержимое локальной папки ~/dev-config
// в одноимённую папку dev-config в домашнем каталоге удалённого пользователя.
func copyDevConfig(username, host, remotePath string) error {
	// Определяем домашнюю директорию текущего пользователя.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %v", err)
	}

	// Путь к локальной папке с конфигами.
	localConfigDir := filepath.Join(home, "dev-config")

	// Целевой путь на удалённом сервере: <remotePath>/dev-config.
	remoteConfigDir := filepath.Join(remotePath, "dev-config")
	return scpDir(localConfigDir, username, host, remoteConfigDir)
}
