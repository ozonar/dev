// Пакет release реализует управление продакшен-релизами: подготовку новой
// папки релиза из результатов сборки и переключение активного релиза через
// симлинк. Конфигурация хранится в release.yml рядом с запуском команды.
package release

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"dev/internal/colors"

	"gopkg.in/yaml.v3"
)

// configFileName — имя конфигурационного файла в рабочей директории.
const configFileName = "release.yml"

// Release описывает папки одного именованного релиза.
type Release struct {
	BuildsFolder       string `yaml:"builds_folder"`
	ReleasesFolder     string `yaml:"releases_folder"`
	CurrentReleaseLink string `yaml:"current_release_folder"`

	// Необязательные параметры имени папки релиза. Пустые значения
	// заменяются дефолтными (release- и 2006-01-02_15-04-05).
	ReleasePrefix     string `yaml:"release_prefix,omitempty"`
	ReleaseTimeFormat string `yaml:"release_time_format,omitempty"`
}

// Config — корневая структура release.yml.
type Config struct {
	Releases map[string]*Release `yaml:"releases"`
}

// defaultTemplate — шаблон release.yml, создаваемый при первом запуске.
const defaultTemplate = `# Production release configuration
#
# Each named release defines three folders:
#   builds_folder            where fresh build artifacts are placed
#   releases_folder          where release-<datetime> archives are stored
#   current_release_folder   symlink path that points to the active release
#
# Optional per-release settings (defaults shown):
#   release_prefix       release-              prefix of release folder names
#   release_time_format  "2006-01-02_15-04-05" time layout inside the name
releases:
  backend:
    builds_folder: ./builds/backend
    releases_folder: ./releases/backend
    current_release_folder: ./current/backend
  frontend:
    builds_folder: ./builds/frontend
    releases_folder: ./releases/frontend
    current_release_folder: ./current/frontend
`

// LoadConfig читает release.yml из директории dir и валидирует его.
// Невозможные состояния (пустой список, отсутствующие пути) превращаются
// в ошибку на этапе загрузки, чтобы нижележащая логика не обрабатывала их.
func LoadConfig(dir string) (*Config, error) {
	path := filepath.Join(dir, configFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(cfg.Releases) == 0 {
		return nil, fmt.Errorf("%s defines no releases", path)
	}
	for name, rel := range cfg.Releases {
		if rel == nil || strings.TrimSpace(rel.BuildsFolder) == "" ||
			strings.TrimSpace(rel.ReleasesFolder) == "" ||
			strings.TrimSpace(rel.CurrentReleaseLink) == "" {
			return nil, fmt.Errorf("release %q: builds_folder, releases_folder and current_release_folder are required", name)
		}
		// Необязательный формат времени должен допускать обратный парсинг,
		// иначе релизы нельзя будет отсортировать по дате.
		if rel.ReleaseTimeFormat != "" && !validTimeFormat(rel.ReleaseTimeFormat) {
			return nil, fmt.Errorf("release %q: invalid release_time_format %q", name, rel.ReleaseTimeFormat)
		}
	}
	return &cfg, nil
}

// validTimeFormat проверяет, что layout позволяет сформировать и разобрать
// дату-время (гарантирует работоспособность сортировки релизов по имени).
func validTimeFormat(layout string) bool {
	probe := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	_, err := time.ParseInLocation(layout, probe.Format(layout), time.Local)
	return err == nil
}

// EnsureConfig гарантирует наличие валидного release.yml. Если файл
// отсутствует — создаётся шаблон, если невалиден — выводится причина;
// в обоих случаях открывается редактор (аналогично команде dev ai),
// после чего конфиг читается повторно.
func EnsureConfig(dir string) (*Config, error) {
	cfg, err := LoadConfig(dir)
	if err == nil {
		return cfg, nil
	}

	path := filepath.Join(dir, configFileName)
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		// Файла нет — создаём шаблон для заполнения.
		if writeErr := os.WriteFile(path, []byte(defaultTemplate), 0644); writeErr != nil {
			return nil, fmt.Errorf("failed to create %s: %w", path, writeErr)
		}
		fmt.Println(colors.Yellow("Created " + path + " from template"))
	} else {
		fmt.Println(colors.Yellow("Invalid " + path + ": " + err.Error()))
	}

	if openErr := openInEditor(path); openErr != nil {
		return nil, openErr
	}
	return LoadConfig(dir)
}

// openInEditor открывает файл в редакторе из переменной $EDITOR
// (по умолчанию nano) с передачей stdin/stdout терминала.
func openInEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}
	// EDITOR может содержать флаги (например "code --wait") — разбиваем.
	parts := strings.Fields(editor)
	args := append(parts[1:], path)

	fmt.Println(colors.Cyan("Opening " + path + " in " + parts[0] + "..."))
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
