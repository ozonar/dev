package docker

import (
	"dev/internal/colors"
	"dev/internal/compose"
	"dev/internal/i18n"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GetServiceStatuses возвращает карту статусов сервисов Docker Compose.
// Ключ - имя сервиса, значение - сырой статус (например, "Up", "Exited", "Failed").
func GetServiceStatuses() (map[string]string, error) {
	if _, err := os.Stat(compose.FileName); os.IsNotExist(err) {
		return nil, nil // нет compose-файла - нет сервисов
	}

	// Получаем имя текущего проекта Docker Compose (по умолчанию - имя текущей директории)
	projectName, err := compose.ProjectName()
	if err != nil {
		// Если не удалось определить проект, возвращаем пустую карту
		return map[string]string{}, nil
	}

	// Получаем JSON всех контейнеров текущего проекта
	cmd := exec.Command("docker", "ps", "-a", "--filter", "label=com.docker.compose.project="+projectName, "--format", "{{json .}}")
	jsonOut, err := cmd.Output()
	if err != nil {
		// Если ошибка (например, docker не доступен), возвращаем пустую карту
		return map[string]string{}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(jsonOut)), "\n")
	statuses := make(map[string]string)

	// Парсим каждую строку JSON
	for _, line := range lines {
		if line == "" {
			continue
		}
		var container map[string]interface{}
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			continue // пропускаем некорректные JSON
		}
		labels, _ := container["Labels"].(string)
		status, _ := container["Status"].(string)
		// Извлекаем имя сервиса из лейблов
		svc := extractServiceFromLabels(labels)
		if svc != "" && status != "" {
			statuses[svc] = status
		}
	}
	return statuses, nil
}

// extractServiceFromLabels извлекает значение com.docker.compose.service из строки лейблов.
// Лейблы имеют формат "key=value,key2=value2"
func extractServiceFromLabels(labels string) string {
	pairs := strings.Split(labels, ",")
	for _, pair := range pairs {
		kv := strings.Split(pair, "=")
		if len(kv) == 2 && strings.TrimSpace(kv[0]) == "com.docker.compose.service" {
			return strings.TrimSpace(kv[1])
		}
	}
	return ""
}

// ColorizeStatus возвращает цветную строку статуса согласно правилам:
// - Зеленый: активен (статус содержит "Up") или запускается ("Starting", "Restarting")
// - Синий: неактивен ("Created", "Paused")
// - Красный: упал с ошибкой ("Exited", "Exit", "Failed", "Error", "Dead")
// - Желтый: неизвестный статус
func ColorizeStatus(status string) string {
	lower := strings.ToLower(status)
	if strings.Contains(lower, "up") || strings.Contains(lower, "starting") || strings.Contains(lower, "restarting") {
		return colors.Green(status)
	}
	if strings.Contains(lower, "created") || strings.Contains(lower, "paused") {
		return colors.Blue(status)
	}
	if strings.Contains(lower, "exited") || strings.Contains(lower, "exit") || strings.Contains(lower, "failed") || strings.Contains(lower, "error") || strings.Contains(lower, "dead") {
		return colors.Red(status)
	}
	return colors.Yellow(status)
}

func ComposeUp() error {
	// Check if compose file exists
	if _, err := os.Stat(compose.FileName); os.IsNotExist(err) {
		return errors.New(i18n.T("docker-compose.yml not found"))
	}

	i18n.Cyan("Running docker-compose up -d...")
	cmd := exec.Command("docker-compose", "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf(i18n.T("docker-compose up failed: %v"), err)
	}

	// Check services status
	i18n.Cyan("Checking services...")
	cmd = exec.Command("docker-compose", "ps", "--services")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf(i18n.T("failed to list services: %v"), err)
	}
	services := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(services) == 0 {
		i18n.Cyan("No services found.")
		return nil
	}

	i18n.Green("Services running:")
	for _, svc := range services {
		if svc == "" {
			continue
		}
		// Get status
		cmd = exec.Command("docker-compose", "ps", "-a", "--filter", "service="+svc, "--format", "{{.Status}}")
		statusOut, _ := cmd.Output()
		status := strings.TrimSpace(string(statusOut))
		if strings.Contains(status, "Up") {
			fmt.Printf("  %s: %s\n", svc, colors.Green("Up"))
		} else {
			fmt.Printf("  %s: %s\n", svc, colors.Red(status))
		}
	}
	return nil
}
