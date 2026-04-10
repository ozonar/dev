package detector

import (
	"dev/internal/common"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	LocationLocal  = "localhost"
	LocationDocker = "docker"
	LocationRemote = "remote"
)

type DatabaseInfo struct {
	Type     string // postgresql, mysql, mongodb, redis
	URL      string // полная строка подключения (если найдена)
	Host     string // хост
	Port     string // порт
	Database string // имя базы данных
	Location string // localhost, docker, remote
}

type ProjectInfo struct {
	Language       string
	Framework      string
	HasEnv         bool
	HasVendor      bool
	DockerServices []string
	MakeCommands   []string
	DevCommands    []string
	CacheDirs      []string
	LogFiles       []string
	Databases      []DatabaseInfo
}

func DetectProject(root string) (*ProjectInfo, error) {
	info := &ProjectInfo{}

	// Detect language/framework
	lang, framework := detectLangFramework(root)
	info.Language = lang
	info.Framework = framework

	// Check .env
	info.HasEnv = common.FileExists(filepath.Join(root, ".env"))

	// Check vendor/composer/node_modules etc
	info.HasVendor = checkVendor(root, framework)

	// Docker services
	info.DockerServices = findDockerServices(root)

	// Make commands
	info.MakeCommands = parseMakefile(root)

	// Dev commands (from package.json, composer.json, etc)
	info.DevCommands = findDevCommands(root, framework)

	// Cache directories
	info.CacheDirs = findCacheDirs(root, framework)

	// Log files
	info.LogFiles = findLogFiles(root)

	// Databases
	info.Databases = detectDatabases(root)

	return info, nil
}

func detectLangFramework(root string) (string, string) {
	// Check for composer.json -> PHP
	if common.FileExists(filepath.Join(root, "composer.json")) {
		// Try to detect framework
		if common.FileExists(filepath.Join(root, "artisan")) {
			return "php", "laravel"
		}
		if common.FileExists(filepath.Join(root, "symfony.lock")) {
			return "php", "symfony"
		}
		if common.FileExists(filepath.Join(root, "bin/console")) {
			return "php", "symfony"
		}
		if common.FileExists(filepath.Join(root, "yii")) {
			return "php", "yii"
		}
		return "php", "generic"
	}
	// Check for go.mod -> Go
	if common.FileExists(filepath.Join(root, "go.mod")) {
		return "go", "go"
	}
	// Check for package.json -> Node.js
	if common.FileExists(filepath.Join(root, "package.json")) {
		// Check for React, Vue, Angular etc via dependencies
		return "javascript", "node"
	}
	// Check for Gemfile -> Ruby (Rails)
	if common.FileExists(filepath.Join(root, "Gemfile")) {
		// Check for Rails
		if common.FileExists(filepath.Join(root, "config/application.rb")) || common.FileExists(filepath.Join(root, "config.ru")) {
			return "ruby", "rails"
		}
		return "ruby", "generic"
	}
	// Check for requirements.txt or pyproject.toml -> Python
	if common.FileExists(filepath.Join(root, "requirements.txt")) || common.FileExists(filepath.Join(root, "pyproject.toml")) {
		// Check for Django
		if common.FileExists(filepath.Join(root, "manage.py")) {
			return "python", "django"
		}
		return "python", "generic"
	}
	// Default
	return "unknown", ""
}

func checkVendor(root, framework string) bool {
	switch framework {
	case "laravel", "symfony", "generic":
		return common.FileExists(filepath.Join(root, "vendor"))
	case "go":
		return common.FileExists(filepath.Join(root, "go.sum"))
	case "node":
		return common.FileExists(filepath.Join(root, "node_modules"))
	default:
		return false
	}
}

func findDockerServices(root string) []string {
	composePath := filepath.Join(root, "docker-compose.yml")
	if !common.FileExists(composePath) {
		return nil
	}
	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var services []string
	inServices := false
	servicesIndent := -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Определяем отступ (количество пробелов в начале строки)
		indent := 0
		for _, ch := range line {
			if ch == ' ' {
				indent++
			} else if ch == '\t' {
				indent += 4 // считаем таб как 4 пробела
			} else {
				break
			}
		}
		if strings.HasPrefix(trimmed, "services:") {
			inServices = true
			servicesIndent = indent
			continue
		}
		if !inServices {
			continue
		}
		// Если отступ меньше или равен отступу services: и строка не пустая,
		// значит, мы вышли из блока services (например, volumes:, networks:)
		if indent <= servicesIndent && trimmed != "" {
			// Проверяем, не является ли это другим top-level ключом
			if strings.Contains(trimmed, ":") {
				break
			}
		}
		// Игнорируем строки, которые начинаются с '-'
		if strings.HasPrefix(trimmed, "-") {
			continue
		}
		// Сервис должен иметь отступ ровно на 2 пробела больше, чем services:
		// (обычно servicesIndent = 0, indent = 2)
		expectedIndent := servicesIndent + 2
		if indent == expectedIndent && strings.Contains(trimmed, ":") {
			// Извлекаем имя сервиса (часть до двоеточия)
			parts := strings.Split(trimmed, ":")
			svc := strings.TrimSpace(parts[0])
			if svc != "" {
				services = append(services, svc)
			}
		}
	}
	return services
}

func parseMakefile(root string) []string {
	makePath := filepath.Join(root, "Makefile")
	if !common.FileExists(makePath) {
		return nil
	}
	data, err := os.ReadFile(makePath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var commands []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ".PHONY:") {
			// Extract phony targets
			parts := strings.Split(trimmed, ":")
			if len(parts) > 1 {
				targets := strings.Fields(parts[1])
				commands = append(commands, targets...)
			}
		}
		// Match target definitions (word:)
		if len(trimmed) > 0 && !strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, ":") && !strings.Contains(trimmed, "=") {
			target := strings.Split(trimmed, ":")[0]
			if !strings.Contains(target, " ") && target != "" {
				commands = append(commands, target)
			}
		}
	}
	return common.Unique(commands)
}

func findDevCommands(root, framework string) []string {
	// For now, return empty
	return nil
}

func findCacheDirs(root, framework string) []string {
	var dirs []string
	switch framework {
	case "laravel", "symfony":
		dirs = append(dirs, filepath.Join(root, "var/cache"))
		dirs = append(dirs, filepath.Join(root, "storage/framework/cache"))
	case "go":
		dirs = append(dirs, filepath.Join(root, "**/*.test"))
	case "node":
		dirs = append(dirs, filepath.Join(root, "node_modules/.cache"))
	case "python":
		dirs = append(dirs, filepath.Join(root, "__pycache__"))
		dirs = append(dirs, filepath.Join(root, "*.pyc"))
	}
	return dirs
}

func findLogFiles(root string) []string {
	var logs []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".log") {
			logs = append(logs, path)
		}
		return nil
	})
	return logs
}

// extractURL находит первую подстроку, соответствующую шаблону URL БД в строке
func extractURL(line string) (string, string) {
	// Регулярное выражение для поиска URL БД
	re := regexp.MustCompile(`(postgresql|mysql|mongodb|redis)://[^\s'"` + "`" + `]+`)
	matches := re.FindStringSubmatch(line)
	if matches == nil {
		return "", ""
	}
	return matches[0], matches[1] // полный URL и тип
}

// detectDatabases ищет строки подключения к БД в .env файлах и других конфигурациях
func detectDatabases(root string) []DatabaseInfo {
	var databases []DatabaseInfo

	// Проверяем .env файл
	envPath := filepath.Join(root, ".env")
	if common.FileExists(envPath) {
		data, err := os.ReadFile(envPath)
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "#") || line == "" {
					continue
				}
				// Пытаемся извлечь URL БД из строки
				url, dbType := extractURL(line)
				if url != "" {
					db := parseConnectionString(url, dbType)
					if db != nil {
						databases = append(databases, *db)
					}
				}
			}
		}
	}

	// TODO: также можно проверить docker-compose.yml на наличие сервисов БД

	return databases
}

// parseConnectionString парсит строку подключения и определяет местоположение
func parseConnectionString(url, dbType string) *DatabaseInfo {
	// Упрощённый парсинг URL
	// Пример: postgresql://user:pass@localhost:5432/dbname
	re := regexp.MustCompile(`^([a-z]+)://(?:([^:@]+)(?::([^@]+))?@)?([^:/]+)(?::(\d+))?(?:/([^?]+))?`)
	matches := re.FindStringSubmatch(url)
	if matches == nil {
		return nil
	}
	// matches[1] - тип (должен совпадать с dbType)
	// matches[4] - хост
	// matches[5] - порт
	// matches[6] - база данных
	host := matches[4]
	if host == "" {
		host = "localhost"
	}
	port := matches[5]
	if port == "" {
		// порты по умолчанию
		switch dbType {
		case "postgresql":
			port = "5432"
		case "mysql":
			port = "3306"
		case "mongodb":
			port = "27017"
		case "redis":
			port = "6379"
		}
	}
	database := matches[6]

	// Определяем местоположение
	location := LocationRemote
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		location = LocationLocal
	} else if strings.Contains(host, "docker") || strings.Contains(host, "container") {
		location = LocationDocker
	} else {
		// Эвристика: если хост не содержит точек и не является IP адресом, то вероятно docker контейнер
		if !strings.Contains(host, ".") && !strings.Contains(host, ":") {
			// Проверяем, не является ли числовым IP (например, "192168")
			// Простая проверка: если host состоит только из цифр и точек, то это IP, но точек нет
			// Считаем docker
			location = LocationDocker
		}
	}

	return &DatabaseInfo{
		Type:     dbType,
		URL:      url,
		Host:     host,
		Port:     port,
		Database: database,
		Location: location,
	}
}
