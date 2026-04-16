package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dev/internal/colors"
)

// GetServiceStatuses возвращает карту статусов сервисов Docker Compose.
// Ключ - имя сервиса, значение - сырой статус (например, "Up", "Exited", "Failed").
func GetServiceStatuses() (map[string]string, error) {
	if _, err := os.Stat("docker-compose.yml"); os.IsNotExist(err) {
		return nil, nil // нет docker-compose.yml - нет сервисов
	}

	// Получаем имя текущего проекта Docker Compose (по умолчанию - имя текущей директории)
	projectName, err := getComposeProjectName()
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

// getComposeProjectName возвращает имя проекта Docker Compose.
// По умолчанию используется имя текущей директории в нижнем регистре (как делает docker-compose).
func getComposeProjectName() (string, error) {
	// Можно также учесть переменную окружения COMPOSE_PROJECT_NAME
	if project := os.Getenv("COMPOSE_PROJECT_NAME"); project != "" {
		return project, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	project := filepath.Base(wd)
	// Docker Compose преобразует имя проекта в нижний регистр и заменяет неалфавитно-цифровые символы на дефисы
	// Для простоты приводим к нижнему регистру, как это делает docker-compose по умолчанию
	return strings.ToLower(project), nil
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
	// Check if docker-compose.yml exists
	if _, err := os.Stat("docker-compose.yml"); os.IsNotExist(err) {
		return fmt.Errorf("docker-compose.yml not found")
	}

	fmt.Println(colors.Cyan("Running docker-compose up -d..."))
	cmd := exec.Command("docker-compose", "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("docker-compose up failed: %v", err)
	}

	// Check services status
	fmt.Println(colors.Cyan("Checking services..."))
	cmd = exec.Command("docker-compose", "ps", "--services")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list services: %v", err)
	}
	services := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(services) == 0 {
		fmt.Println(colors.Cyan("No services found."))
		return nil
	}

	fmt.Println(colors.Green("Services running:"))
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
