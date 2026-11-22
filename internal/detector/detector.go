package detector

import (
	"dev/internal/common"
	"dev/internal/compose"
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
	Databases       []DatabaseInfo
}

func DetectProject(root string) (*ProjectInfo, error) {
	info := &ProjectInfo{}

	// Определяем язык и фреймворк проекта.
	lang, framework := detectLangFramework(root)
	info.Language = lang
	info.Framework = framework
	info.LanguageVersion = detectLanguageVersion(root, lang)

	// Проверяем наличие .env.
	info.HasEnv = common.FileExists(filepath.Join(root, common.EnvFile))

	// Проверяем наличие вендорных зависимостей (vendor, node_modules и т.п.).
	info.HasVendor = checkVendor(root, framework)

	// Сервисы из docker-compose.
	info.DockerServices = findDockerServices(root)

	// Команды из Makefile.
	info.MakeCommands = parseMakefile(root)

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
	// composer.json -> PHP.
	if common.FileExists(filepath.Join(root, common.ComposerFile)) {
		// Пытаемся определить фреймворк по маркерным файлам.
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
	// go.mod -> Go.
	if common.FileExists(filepath.Join(root, common.GoModFile)) {
		return "go", "go"
	}
	// package.json -> Node.js.
	if common.FileExists(filepath.Join(root, common.PackageFile)) {
		return "javascript", "node"
	}
	// Gemfile -> Ruby (Rails).
	if common.FileExists(filepath.Join(root, "Gemfile")) {
		if common.FileExists(filepath.Join(root, "config/application.rb")) || common.FileExists(filepath.Join(root, "config.ru")) {
			return "ruby", "rails"
		}
		return "ruby", "generic"
	}
	// requirements.txt или pyproject.toml -> Python.
	if common.FileExists(filepath.Join(root, common.RequirementsFile)) || common.FileExists(filepath.Join(root, common.PyProjectFile)) {
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

// extensionLanguages — сопоставление расширения файла с языком проекта.
var extensionLanguages = map[string]string{
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
	".cjs":  "javascript",
	".ts":   "javascript",
	".tsx":  "javascript",
}

// nonCodeExtensions — расширения, которые участвуют в детекции языка проекта,
// но не являются исходным кодом для линтеров.
var nonCodeExtensions = map[string]bool{
	".html": true,
	".htm":  true,
	".twig": true,
}

// LanguageByExtension определяет язык по расширению файла.
func LanguageByExtension(ext string) (string, bool) {
	lang, ok := extensionLanguages[strings.ToLower(ext)]
	return lang, ok
}

// LintLanguageByExtension определяет язык, для которого существуют линтеры.
// Расширения шаблонов и разметки исключаются.
func LintLanguageByExtension(ext string) (string, bool) {
	ext = strings.ToLower(ext)
	if nonCodeExtensions[ext] {
		return "", false
	}
	return LanguageByExtension(ext)
}

// detectLangByExtension пытается определить язык проекта по расширениям
// файлов, лежащих непосредственно в корне (без рекурсивного обхода).
// Возвращает язык и true, если какой-то из маркерных файлов найден.
func detectLangByExtension(root string) (string, bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if lang, ok := LanguageByExtension(filepath.Ext(entry.Name())); ok {
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
	data, err := os.ReadFile(filepath.Join(root, common.ComposerFile))
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
	data, err := os.ReadFile(filepath.Join(root, common.GoModFile))
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
	data, err := os.ReadFile(filepath.Join(root, common.PackageFile))
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
	data, err := os.ReadFile(filepath.Join(root, common.PyProjectFile))
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

	// Вторая часть, если она задана, должна быть числовой: "8.x" не является
	// конкретной версией, в отличие от "8.4.*".
	rawParts := strings.SplitN(trimmed, ".", 3)
	if len(rawParts) >= 2 {
		if _, err := strconv.Atoi(strings.TrimSpace(rawParts[1])); err != nil {
			return ""
		}
	}
	parts := common.NumericParts(trimmed)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		// Только major-версия (например "18") — дополняем минорную нулём.
		return strconv.Itoa(parts[0]) + ".0"
	}
	return strconv.Itoa(parts[0]) + "." + strconv.Itoa(parts[1])
}

func checkVendor(root, framework string) bool {
	switch framework {
	case "laravel", "symfony", "generic":
		return common.FileExists(filepath.Join(root, "vendor"))
	case "go":
		return common.FileExists(filepath.Join(root, common.GoSumFile))
	case "node":
		return common.FileExists(filepath.Join(root, "node_modules"))
	default:
		return false
	}
}

func findDockerServices(root string) []string {
	parsed := compose.ReadServices(root)
	if len(parsed) == 0 {
		return nil
	}
	services := make([]string, 0, len(parsed))
	for _, svc := range parsed {
		services = append(services, svc.Name)
	}
	return services
}

func parseMakefile(root string) []string {
	makePath := filepath.Join(root, common.MakefileName)
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
			// Извлекаем phony-цели.
			parts := strings.Split(trimmed, ":")
			if len(parts) > 1 {
				targets := strings.Fields(parts[1])
				commands = append(commands, targets...)
			}
		}
		// Обрабатываем определения целей вида "имя:".
		if len(trimmed) > 0 && !strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, ":") && !strings.Contains(trimmed, "=") {
			target := strings.Split(trimmed, ":")[0]
			if !strings.Contains(target, " ") && target != "" {
				commands = append(commands, target)
			}
		}
	}
	return common.Unique(commands)
}

// extractURL находит первую подстроку, соответствующую шаблону URL БД в строке.
func extractURL(line string) (string, string) {
	// Регулярное выражение для поиска URL БД.
	re := regexp.MustCompile(`(postgresql|mysql|mongodb|redis)://[^\s'"` + "`" + `]+`)
	matches := re.FindStringSubmatch(line)
	if matches == nil {
		return "", ""
	}
	return matches[0], matches[1] // полный URL и тип
}

// detectDatabases ищет строки подключения к БД в .env файлах и других конфигурациях.
func detectDatabases(root string) []DatabaseInfo {
	var databases []DatabaseInfo

	// Проверяем .env файл.
	envPath := filepath.Join(root, common.EnvFile)
	if common.FileExists(envPath) {
		data, err := os.ReadFile(envPath)
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "#") || line == "" {
					continue
				}
				// Пытаемся извлечь URL БД из строки.
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

	return databases
}

// parseConnectionString парсит строку подключения и определяет местоположение.
func parseConnectionString(url, dbType string) *DatabaseInfo {
	// Упрощённый парсинг URL: postgresql://user:pass@localhost:5432/dbname
	re := regexp.MustCompile(`^([a-z]+)://(?:([^:@]+)(?::([^@]+))?@)?([^:/]+)(?::(\d+))?(?:/([^?]+))?`)
	matches := re.FindStringSubmatch(url)
	if matches == nil {
		return nil
	}
	// matches[1] — тип (должен совпадать с dbType),
	// matches[4] — хост, matches[5] — порт, matches[6] — база данных.
	host := matches[4]
	if host == "" {
		host = "localhost"
	}
	port := matches[5]
	if port == "" {
		// Порты по умолчанию.
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

	// Определяем местоположение.
	location := LocationRemote
	if common.IsLocalHost(host) {
		location = LocationLocal
	} else if strings.Contains(host, "docker") || strings.Contains(host, "container") {
		location = LocationDocker
	} else if !strings.Contains(host, ".") && !strings.Contains(host, ":") {
		// Эвристика: имя без точек и двоеточий (например имя compose-сервиса)
		// считаем docker-контейнером.
		location = LocationDocker
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
