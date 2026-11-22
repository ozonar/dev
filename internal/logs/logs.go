package logs

import (
	"dev/internal/common"
	"dev/internal/compose"
	"dev/internal/detector"
	"dev/internal/toolchain"
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
	projectName, err := compose.ProjectName()
	if err != nil || projectName == "" {
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

func PHPFPMLogGlobs() []string {
	return []string{
		"/var/log/php*-fpm.log",
		"/var/log/php-fpm/*.log",
	}
}

// OpenLogInLnav открывает лог в lnav. Для docker-контейнеров используется
// docker logs -f, для файлов — lnav. Путь к lnav определяется через toolchain:
// используется системный lnav, либо lnav скачивается в dev-command.
func OpenLogInLnav(path string, logType string) error {
	var cmd *exec.Cmd

	if logType == "docker" {
		cmd = exec.Command("docker", "logs", "-f", path)
	} else {
		lnav, err := toolchain.ResolveLnav()
		if err != nil {
			return err
		}
		cmd = exec.Command(lnav, path)
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

	// 1. Системные пути PHP-FPM логов.
	for _, glob := range PHPFPMLogGlobs() {
		matches, err := filepath.Glob(glob)
		if err != nil {
			continue
		}
		for _, m := range matches {
			addEntry(m)
		}
	}

	// 2. Docker контейнеры с PHP-FPM: сервисы compose, использующие образ php-fpm.
	for _, svc := range compose.ReadServices(projectRoot) {
		image := strings.ToLower(svc.Image)
		if !strings.Contains(image, "php") || !strings.Contains(image, "fpm") {
			continue
		}
		cmd := exec.Command("docker", "ps", "--format", "{{.Names}}", "--filter", "name="+svc.Name)
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		containerName := strings.TrimSpace(string(out))
		if containerName == "" {
			continue
		}
		entries = append(entries, LogEntry{
			Path: containerName,
			Type: "docker",
		})
	}

	return entries
}
