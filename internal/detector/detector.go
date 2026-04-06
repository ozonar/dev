package detector

import (
	"os"
	"path/filepath"
	"strings"
)

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
}

func DetectProject(root string) (*ProjectInfo, error) {
	info := &ProjectInfo{}

	// Detect language/framework
	lang, framework := detectLangFramework(root)
	info.Language = lang
	info.Framework = framework

	// Check .env
	info.HasEnv = fileExists(filepath.Join(root, ".env"))

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

	return info, nil
}

func detectLangFramework(root string) (string, string) {
	// Check for composer.json -> PHP
	if fileExists(filepath.Join(root, "composer.json")) {
		// Try to detect framework
		if fileExists(filepath.Join(root, "artisan")) {
			return "php", "laravel"
		}
		if fileExists(filepath.Join(root, "symfony.lock")) {
			return "php", "symfony"
		}
		if fileExists(filepath.Join(root, "yii")) {
			return "php", "yii"
		}
		return "php", "generic"
	}
	// Check for go.mod -> Go
	if fileExists(filepath.Join(root, "go.mod")) {
		return "go", "go"
	}
	// Check for package.json -> Node.js
	if fileExists(filepath.Join(root, "package.json")) {
		// Check for React, Vue, Angular etc via dependencies
		return "javascript", "node"
	}
	// Check for requirements.txt or pyproject.toml -> Python
	if fileExists(filepath.Join(root, "requirements.txt")) || fileExists(filepath.Join(root, "pyproject.toml")) {
		return "python", "django" // generic
	}
	// Default
	return "unknown", ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func checkVendor(root, framework string) bool {
	switch framework {
	case "laravel", "symfony", "generic":
		return fileExists(filepath.Join(root, "vendor"))
	case "go":
		return fileExists(filepath.Join(root, "go.sum"))
	case "node":
		return fileExists(filepath.Join(root, "node_modules"))
	default:
		return false
	}
}

func findDockerServices(root string) []string {
	composePath := filepath.Join(root, "docker-compose.yml")
	if !fileExists(composePath) {
		return nil
	}
	// Simple parsing: look for services: block
	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var services []string
	inServices := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "services:") {
			inServices = true
			continue
		}
		if inServices && strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, " ") && !strings.HasPrefix(trimmed, "\t") {
			// New top-level key (maybe end of services)
			if strings.Contains(trimmed, "volumes:") || strings.Contains(trimmed, "networks:") {
				break
			}
			// Service name is before colon
			parts := strings.Split(trimmed, ":")
			if len(parts) > 0 {
				svc := strings.TrimSpace(parts[0])
				if svc != "" {
					services = append(services, svc)
				}
			}
		}
	}
	return services
}

func parseMakefile(root string) []string {
	makePath := filepath.Join(root, "Makefile")
	if !fileExists(makePath) {
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
	return unique(commands)
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

func unique(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
