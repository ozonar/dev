// Package recipe реализует рецепты dev: наборы команд, хранящиеся в
// отдельном репозитории рецептов. Источник — локальная папка или HTTP(S)-база,
// где лежат repository.yml (индекс) и recipes/<name>.yml (файлы рецептов).
package recipe

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dev/internal/custom"
	"dev/internal/i18n"
	"dev/internal/selfconfig"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultRepo — база репозитория рецептов по умолчанию (raw-доступ GitHub).
	DefaultRepo = "https://raw.githubusercontent.com/ozonar/dev-recipe/main"
	// IndexFile — имя индекса рецептов в корне репозитория.
	IndexFile = "repository.yml"
	// RecipesDir — папка с файлами рецептов в репозитории.
	RecipesDir = "recipes"
	// httpTimeout — таймаут скачивания файлов рецептов.
	httpTimeout = 30 * time.Second
)

// Entry описывает рецепт в индексе repository.yml.
type Entry struct {
	Description string `yaml:"description,omitempty"`
	File        string `yaml:"file"`
}

// Repository — содержимое repository.yml.
type Repository struct {
	Recipes map[string]Entry `yaml:"recipes"`
}

// recipeFile — файл рецепта в формате custom-команд: верхний уровень
// commands, имя команды — ключ. Один файл может содержать несколько команд.
type recipeFile struct {
	Commands map[string]custom.Command `yaml:"commands"`
}

// ListItem — рецепт для вывода списка.
type ListItem struct {
	Name        string
	Description string
}

// Prepared — рецепт, готовый к запуску.
type Prepared struct {
	Name        string
	Description string
	CachedAt    time.Time
	Command     custom.Command
	FromCache   bool
}

// NotFoundError — рецепт с запрошенным именем не найден.
type NotFoundError struct {
	Query   string
	Matches []ListItem
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("recipe %q not found", e.Query)
}

// RepoURL возвращает базу репозитория рецептов. Ключ RECIPE_REPO читается
// из окружения или main.conf без учёта регистра. Если ключ не задан —
// возвращается DefaultRepo.
func RepoURL() string {
	if v := selfconfig.Value("RECIPE_REPO"); v != "" {
		return resolvePath(v)
	}
	return DefaultRepo
}

// CacheDir возвращает абсолютный путь к папке скачанных рецептов.
// Папка находится рядом с активным конфигом (selfconfig.Dir), поэтому
// при системном конфиге используется /etc/dev-command/recipes.
func CacheDir() string {
	return filepath.Join(selfconfig.Dir(), RecipesDir)
}

// resolvePath заменяет префикс "~/" на домашнюю директорию пользователя.
func resolvePath(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// validateName проверяет, что имя рецепта безопасно использовать как имя
// файла в кэше и что оно не содержит разделителей пути.
func validateName(name string) error {
	if name == "" {
		return errors.New(i18n.T("recipe name is empty"))
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf(i18n.T("invalid recipe name %q"), name)
	}
	return nil
}

// validateRelPath проверяет, что путь из индекса остаётся внутри репозитория.
func validateRelPath(rel string) error {
	if strings.TrimSpace(rel) == "" {
		return errors.New(i18n.T("recipe file path is empty"))
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf(i18n.T("invalid recipe file path %q"), rel)
	}
	return nil
}

// isHTTP сообщает, что источник рецептов — HTTP(S)-база.
func isHTTP(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}

// localPath приводит file:// и "~/" к локальному пути.
func localPath(source string) string {
	return resolvePath(strings.TrimPrefix(source, "file://"))
}

// joinURL склеивает базовый URL и относительный путь.
func joinURL(base, rel string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(rel, "/")
}

// fetchHTTP скачивает файл по HTTP(S).
func fetchHTTP(url string) ([]byte, error) {
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(i18n.T("unexpected HTTP status %d for %s"), resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// readSource читает файл рецепта из локальной папки или по HTTP(S).
func readSource(source, rel string) ([]byte, error) {
	if err := validateRelPath(rel); err != nil {
		return nil, err
	}
	if isHTTP(source) {
		return fetchHTTP(joinURL(source, rel))
	}
	return os.ReadFile(filepath.Join(localPath(source), filepath.FromSlash(rel)))
}

// LoadRepository читает repository.yml из источника рецептов.
func LoadRepository() (*Repository, error) {
	data, err := readSource(RepoURL(), IndexFile)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("failed to read recipe index: %w"), err)
	}
	repo := &Repository{}
	if err := yaml.Unmarshal(data, repo); err != nil {
		return nil, fmt.Errorf(i18n.T("failed to parse recipe index: %w"), err)
	}
	if repo.Recipes == nil {
		repo.Recipes = map[string]Entry{}
	}
	return repo, nil
}

// Names возвращает отсортированный список имён рецептов.
func (r *Repository) Names() []string {
	names := make([]string, 0, len(r.Recipes))
	for name := range r.Recipes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// List возвращает рецепты в порядке имён.
func (r *Repository) List() []ListItem {
	names := r.Names()
	items := make([]ListItem, 0, len(names))
	for _, name := range names {
		items = append(items, ListItem{
			Name:        name,
			Description: r.Recipes[name].Description,
		})
	}
	return items
}

// Match возвращает рецепты, имена которых содержат подстроку query
// без учёта регистра.
func (r *Repository) Match(query string) []ListItem {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	var matches []ListItem
	for _, item := range r.List() {
		if strings.Contains(strings.ToLower(item.Name), query) {
			matches = append(matches, item)
		}
	}
	return matches
}

// List читает свежий индекс и возвращает список рецептов.
func List() ([]ListItem, error) {
	repo, err := LoadRepository()
	if err != nil {
		return nil, err
	}
	return repo.List(), nil
}

// Prepare возвращает рецепт по имени. Без force уже скачанный рецепт берётся
// из кэша; при отсутствии кэша или с force рецепт читается из источника
// и сохраняется в кэш.
func Prepare(name string, force bool) (*Prepared, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return nil, err
	}

	cachePath := filepath.Join(CacheDir(), name+".yml")

	if !force {
		if prepared, err := readCached(name, cachePath); err == nil {
			return prepared, nil
		}
	}

	repo, err := LoadRepository()
	if err != nil {
		return nil, err
	}

	entry, ok := repo.Recipes[name]
	if !ok {
		return nil, &NotFoundError{Query: name, Matches: repo.Match(name)}
	}

	data, err := readSource(RepoURL(), entry.File)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("failed to read recipe %q: %w"), name, err)
	}
	cmd, err := parseRecipe(data, name)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("failed to parse recipe %q: %w"), name, err)
	}

	if err := os.MkdirAll(CacheDir(), 0755); err != nil {
		return nil, fmt.Errorf(i18n.T("failed to create recipe cache directory: %w"), err)
	}
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return nil, fmt.Errorf(i18n.T("failed to cache recipe %q: %w"), name, err)
	}

	info, err := os.Stat(cachePath)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("failed to stat cached recipe %q: %w"), name, err)
	}

	return &Prepared{
		Name:        name,
		Description: cmd.Description,
		CachedAt:    info.ModTime(),
		Command:     *cmd,
		FromCache:   false,
	}, nil
}

// readCached читает рецепт из кэша.
func readCached(name, path string) (*Prepared, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cmd, err := parseRecipe(data, name)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &Prepared{
		Name:        name,
		Description: cmd.Description,
		CachedAt:    info.ModTime(),
		Command:     *cmd,
		FromCache:   true,
	}, nil
}

// parseRecipe разбирает файл рецепта в формате custom-команд и выбирает
// команду с именем name.
func parseRecipe(data []byte, name string) (*custom.Command, error) {
	file := &recipeFile{}
	if err := yaml.Unmarshal(data, file); err != nil {
		return nil, err
	}
	cmd, ok := file.Commands[name]
	if !ok {
		return nil, fmt.Errorf(i18n.T("recipe %q is not defined"), name)
	}
	if len(cmd.Subcommands) == 0 {
		return nil, errors.New(i18n.T("recipe has no subcommands"))
	}
	if cmd.Name != "" && cmd.Name != name {
		return nil, fmt.Errorf(i18n.T("recipe file name %q does not match recipe name %q"), cmd.Name, name)
	}
	return &cmd, nil
}

// Execute выполняет рецепт в заданном контексте.
func Execute(prepared *Prepared, ctx custom.Context) error {
	cfg := &custom.Config{
		Commands: map[string]custom.Command{prepared.Name: prepared.Command},
	}
	found, err := cfg.RunCommand(prepared.Name, ctx)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf(i18n.T("recipe %q is not available in %s"), prepared.Name, ctx.Dir)
	}
	return nil
}
