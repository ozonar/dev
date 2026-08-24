package detector

import (
	"dev/internal/common"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	Language        string
	LanguageVersion string
	Framework       string
	PublicDir       string // публичная директория проекта (для PHP: public/ или web/; для Go: папка с main.go)
	HasEnv          bool
	HasVendor       bool
	DockerServices  []string
	MakeCommands    []string
	DevCommands     []string
	CacheDirs       []string
	LogFiles        []string
	Databases       []DatabaseInfo
}

func DetectProject(root string) (*ProjectInfo, error) {
	info := &ProjectInfo{}

	// Detect language/framework
	lang, framework := detectLangFramework(root)
	info.Language = lang
	info.Framework = framework
	info.LanguageVersion = detectLanguageVersion(root, lang)

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

	// Публичная директория
	info.PublicDir = detectPublicDir(root, framework)

	// Databases
	info.Databases = detectDatabases(root)

	return info, nil
}

// detectPublicDir определяет публичную директорию проекта.
func detectPublicDir(root, framework string) string {
	switch framework {
	case "symfony", "laravel", "yii", "generic":
		// Современная структура PHP: public/index.php
		if common.FileExists(filepath.Join(root, "public", "index.php")) {
			abs, _ := filepath.Abs(filepath.Join(root, "public"))
			return abs
		}
		// Старая структура PHP (Symfony 3 и ранее, Yii2 Basic): web/index.php
		if common.FileExists(filepath.Join(root, "web", "index.php")) {
			abs, _ := filepath.Abs(filepath.Join(root, "web"))
			return abs
		}
		// Yii2 Advanced: frontend/web или backend/web
		if framework == "yii" {
			if common.FileExists(filepath.Join(root, "frontend", "web", "index.php")) {
				abs, _ := filepath.Abs(filepath.Join(root, "frontend", "web"))
				return abs
			}
			if common.FileExists(filepath.Join(root, "backend", "web", "index.php")) {
				abs, _ := filepath.Abs(filepath.Join(root, "backend", "web"))
				return abs
			}
		}
	case "go":
		// Для Go — директория, содержащая main.go
		mains, err := common.FindGoMain(root, common.FindGoMainOptions{
			SearchInCmdFirst: false,
			ExcludeDirs:      []string{},
			OnlyMainGo:       false,
		})
		if err == nil && len(mains) > 0 {
			// Первый main.go находится относительно корня проекта,
			// поэтому строим абсолютный путь через filepath.Join(root, ...)
			dir := filepath.Dir(mains[0])
			abs, _ := filepath.Abs(filepath.Join(root, dir))
			return abs
		}
	}
	// Для остальных случаев и при отсутствии публичной директории
	// возвращаем корневую папку проекта
	abs, _ := filepath.Abs(root)
	return abs
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
	// Fallback: если стандартные маркеры не найдены, пытаемся определить язык
	// по расширению файлов в корне проекта (например .py -> python, .go -> go).
	if lang, ok := detectLangByExtension(root); ok {
		return lang, "generic"
	}
	// Default
	return "unknown", ""
}

// detectLangByExtension пытается определить язык проекта по расширениям
// файлов, лежащих непосредственно в корне (без рекурсивного обхода).
// Возвращает язык и true, если какой-то из маркерных файлов найден.
func detectLangByExtension(root string) (string, bool) {
	// Карта расширение -> язык.
	extLang := map[string]string{
		".py":   "python",
		".go":   "go",
		".rb":   "ruby",
		".php":  "php",
		".twig": "php",
		".html": "php",
		".htm":  "php",
		".js":   "javascript",
		".jsx":  "javascript",
		".mjs":  "javascript",
		".ts":   "javascript",
		".tsx":  "javascript",
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if lang, ok := extLang[ext]; ok {
			return lang, true
		}
	}
	return "", false
}

// detectLanguageVersion определяет требуемую версию языка из конфигурации проекта.
//   - php: composer.json (require.php), например "^8.1"
//   - go: go.mod (директива go), например "1.22"
//   - javascript: package.json (engines.node), например ">=18"
//   - python: pyproject.toml (requires-python) или .python-version
//   - ruby: .ruby-version
//
// Если определить версию не удалось, возвращается пустая строка.
func detectLanguageVersion(root, language string) string {
	switch language {
	case "php":
		return phpVersionFromComposer(root)
	case "go":
		return goVersionFromMod(root)
	case "javascript":
		return nodeVersionFromPackage(root)
	case "python":
		return pythonVersion(root)
	case "ruby":
		return rubyVersion(root)
	default:
		return ""
	}
}

// phpVersionFromComposer извлекает версию PHP из composer.json (require.php).
func phpVersionFromComposer(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	return normalizeMajorMinor(cfg.Require["php"])
}

// goVersionFromMod извлекает версию Go из go.mod (директива go).
func goVersionFromMod(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "go" {
			return normalizeMajorMinor(fields[1])
		}
	}
	return ""
}

// nodeVersionFromPackage извлекает версию Node из package.json (engines.node).
func nodeVersionFromPackage(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Engines map[string]string `json:"engines"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	return normalizeMajorMinor(cfg.Engines["node"])
}

// pythonVersion определяет версию Python из pyproject.toml (requires-python)
// или из .python-version.
func pythonVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "requires-python") && strings.Contains(trimmed, "=") {
				val := strings.TrimSpace(strings.SplitN(trimmed, "=", 2)[1])
				val = strings.Trim(val, `"'`)
				if v := normalizeMajorMinor(val); v != "" {
					return v
				}
			}
		}
	}
	// Fallback на .python-version (например "3.11").
	versionFile, err := os.ReadFile(filepath.Join(root, ".python-version"))
	if err == nil {
		return normalizeMajorMinor(strings.TrimSpace(string(versionFile)))
	}
	return ""
}

// rubyVersion определяет версию Ruby из .ruby-version (например "3.2.2").
func rubyVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, ".ruby-version"))
	if err != nil {
		return ""
	}
	return normalizeMajorMinor(strings.TrimSpace(string(data)))
}

// normalizeMajorMinor приводит строку версии к формату major.minor.
// Например: "^8.1" -> "8.1", ">=8.2" -> "8.2", "8.4.*" -> "8.4", "~8.3.0" -> "8.3",
// ">=18" -> "18.0" и т.д. Если извлечь не удалось, возвращается пустая строка.
func normalizeMajorMinor(constraint string) string {
	trimmed := strings.TrimSpace(constraint)
	// Отбрасываем операторы сравнения и логические связки в начале.
	for _, prefix := range []string{"^", "~", ">=", ">", "<=", "<", "=", "!=", "||", "dev-"} {
		trimmed = strings.TrimPrefix(trimmed, prefix)
	}
	trimmed = strings.TrimSpace(trimmed)

	// Отбрасываем все, что идёт после пробела (например "|| ^8.3").
	if idx := strings.IndexAny(trimmed, " |,"); idx >= 0 {
		trimmed = trimmed[:idx]
	}

	// Берём первые две числовые части major.minor.
	parts := strings.SplitN(trimmed, ".", 3)
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return ""
	}
	if len(parts) == 1 {
		// Только major-версия (например "18") — дополняем минорную нулём.
		return parts[0] + ".0"
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return ""
	}
	return parts[0] + "." + parts[1]
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
