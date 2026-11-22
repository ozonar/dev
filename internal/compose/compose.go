// Package compose содержит единые правила работы с docker-compose файлами:
// имя файла, имя проекта и разбор списка сервисов. Все модули, которым нужны
// эти данные, используют пакет вместо собственных реализаций.
package compose

import (
	"os"
	"path/filepath"
	"strings"
)

// FileName — имя файла docker-compose в корне проекта.
const FileName = "docker-compose.yml"

// Service описывает сервис из docker-compose файла.
type Service struct {
	Name  string
	Image string
}

// ProjectName возвращает имя проекта Docker Compose: переменная окружения
// COMPOSE_PROJECT_NAME либо имя текущей директории в нижнем регистре.
func ProjectName() (string, error) {
	if project := os.Getenv("COMPOSE_PROJECT_NAME"); project != "" {
		return project, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return strings.ToLower(filepath.Base(wd)), nil
}

// ReadServices читает docker-compose файл из root и возвращает его сервисы.
// Отсутствие файла не является ошибкой — возвращается nil.
func ReadServices(root string) []Service {
	data, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		return nil
	}
	return ParseServices(data)
}

// ParseServices извлекает сервисы из содержимого docker-compose файла.
// Учитываются только записи верхнего блока services; для каждого сервиса
// определяется имя и, если задан, образ (image).
func ParseServices(data []byte) []Service {
	var services []Service
	inServices := false
	servicesIndent := -1
	current := -1

	for _, raw := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := indentWidth(raw)

		if !inServices {
			if strings.HasPrefix(trimmed, "services:") {
				inServices = true
				servicesIndent = indent
			}
			continue
		}

		// Строка на уровне services: или выше — блок сервисов закончился.
		if indent <= servicesIndent {
			break
		}

		// Поля сервиса (image и другие) находятся глубже имени сервиса.
		if current >= 0 && indent > servicesIndent+2 && strings.HasPrefix(trimmed, "image:") {
			image := strings.TrimSpace(strings.TrimPrefix(trimmed, "image:"))
			services[current].Image = strings.Trim(image, `"'`)
			continue
		}

		// Имя сервиса — строка ровно на два пробела глубже services:.
		if indent == servicesIndent+2 && strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
			name := strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
			if name == "" {
				continue
			}
			services = append(services, Service{Name: name})
			current = len(services) - 1
		}
	}
	return services
}

// indentWidth считает отступ строки в пробелах, считая таб за 4 пробела.
func indentWidth(line string) int {
	n := 0
	for _, ch := range line {
		switch ch {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}
