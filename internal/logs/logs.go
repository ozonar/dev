package logs

import (
	"dev/internal/common"
	"dev/internal/detector"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type LogEntry struct {
	Path string
	Type string // "file", "docker"
}

func FindLogs(projectRoot string) ([]LogEntry, error) {
	var entries []LogEntry

	// 1. Find *.log files (с пропуском node_modules, vendor, .git и т.д.)
	err := common.WalkWithExclusions(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".log") {
			rel, _ := filepath.Rel(projectRoot, path)
			entries = append(entries, LogEntry{
				Path: rel,
				Type: "file",
			})
		}
		return nil
	}, nil)
	if err != nil {
		return nil, err
	}

	// 2. Docker logs (simplified: check for running containers)
	entries = append(entries, findDockerLogs()...)

	// 3. PHP-FPM logs (если проект на PHP)
	info, err := detector.DetectProject(projectRoot)
	if err == nil && info.Language == "php" {
		phpfpmlogs := findPHPFPMLogs(projectRoot)
		entries = append(entries, phpfpmlogs...)
	}

	return entries, nil
}

func findDockerLogs() []LogEntry {
	// Определяем имя проекта Docker Compose (по умолчанию — имя текущей директории)
	projectName := getComposeProjectName()
	if projectName == "" {
		return nil
	}

	// Получаем контейнеры только текущего compose-проекта
	cmd := exec.Command("docker", "ps", "--filter", "label=com.docker.compose.project="+projectName, "--format", "{{.Names}}")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var entries []LogEntry
	for _, name := range lines {
		if name == "" {
			continue
		}
		entries = append(entries, LogEntry{
			Path: name,
			Type: "docker",
		})
	}
	return entries
}

// getComposeProjectName возвращает имя проекта Docker Compose.
// По умолчанию используется имя текущей рабочей директории в нижнем регистре.
func getComposeProjectName() string {
	// Можно также учесть переменную окружения COMPOSE_PROJECT_NAME
	if project := os.Getenv("COMPOSE_PROJECT_NAME"); project != "" {
		return project
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	project := filepath.Base(wd)
	return strings.ToLower(project)
}

// hasLnav проверяет, установлен ли lnav в системе
func hasLnav() bool {
	_, err := exec.LookPath("lnav")
	return err == nil
}

// OpenLogInLnav открывает лог в lnav или альтернативном просмотрщике (less/tail).
// Если lnav не установлен, использует less для файлов и tail -f для docker.
func OpenLogInLnav(path string, logType string) error {
	var cmd *exec.Cmd

	if logType == "docker" {
		// Для docker логов всегда используем docker logs -f, но если lnav нет — через less
		if hasLnav() {
			cmd = exec.Command("docker", "logs", "-f", path)
		} else {
			// docker logs --tail=100 -f | less
			cmd = exec.Command("bash", "-c", fmt.Sprintf("docker logs --tail=100 -f %s 2>&1 | less -R", path))
		}
	} else {
		if hasLnav() {
			cmd = exec.Command("lnav", path)
		} else {
			// Используем less с опциями, удобными для логов
			cmd = exec.Command("less", "-R", "+F", "-S", path)
		}
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findPHPFPMLogs ищет логи PHP-FPM: системные (/var/log/php*-fpm.log, /var/log/php-fpm/*.log)
// и docker-контейнеры с PHP-FPM.
func findPHPFPMLogs(projectRoot string) []LogEntry {
	var entries []LogEntry
	seen := make(map[string]bool) // для избежания дубликатов

	addEntry := func(path string) {
		if path == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if seen[abs] {
			return
		}
		seen[abs] = true
		// Проверяем, существует ли файл
		if _, err := os.Stat(abs); err != nil {
			return
		}
		entries = append(entries, LogEntry{
			Path: abs,
			Type: "file",
		})
	}

	// 1. Системные пути PHP-FPM логов — ищем по шаблону /var/log/php*-fpm.log
	//    и /var/log/php-fpm/*.log
	systemGlobs := []string{
		"/var/log/php*-fpm.log",
		"/var/log/php-fpm/*.log",
	}
	for _, glob := range systemGlobs {
		matches, err := filepath.Glob(glob)
		if err != nil {
			continue
		}
		for _, m := range matches {
			addEntry(m)
		}
	}

	// 2. Docker контейнеры с PHP-FPM
	// Проверяем, есть ли docker-compose.yml и ищем сервисы с php-fpm
	composePath := filepath.Join(projectRoot, "docker-compose.yml")
	if composeData, err := os.ReadFile(composePath); err == nil {
		composeContent := string(composeData)
		lines := strings.Split(composeContent, "\n")
		inServices := false
		currentService := ""
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "services:") {
				inServices = true
				continue
			}
			if !inServices || trimmed == "" {
				continue
			}
			// Если строка не начинается с пробела — вышли из services
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
				if strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "#") {
					// Это новый top-level ключ, выходим
					break
				}
			}
			// Определяем имя сервиса (строка с отступом 2 пробела, заканчивается на ":")
			if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":") {
				currentService = strings.TrimSuffix(trimmed, ":")
				continue
			}
			// Проверяем, использует ли сервис php-fpm образ
			if strings.Contains(trimmed, "image:") && strings.Contains(strings.ToLower(trimmed), "php") && strings.Contains(strings.ToLower(trimmed), "fpm") {
				if currentService != "" {
					// Проверяем, запущен ли контейнер
					cmd := exec.Command("docker", "ps", "--format", "{{.Names}}", "--filter", fmt.Sprintf("name=%s", currentService))
					if out, err := cmd.Output(); err == nil {
						containerName := strings.TrimSpace(string(out))
						if containerName != "" {
							entries = append(entries, LogEntry{
								Path: containerName,
								Type: "docker",
							})
						}
					}
				}
			}
		}
	}

	return entries
}
