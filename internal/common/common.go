package common

import (
	"bufio"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"dev/internal/i18n"
)

// SkipDirs — список директорий, которые нужно пропускать при рекурсивном обходе,
// чтобы избежать долгого сканирования больших папок (node_modules, vendor, .git и т.д.)
var SkipDirs = map[string]bool{
	"node_modules":     true,
	"vendor":           true,
	".git":             true,
	".venv":            true,
	"venv":             true,
	"env":              true,
	"__pycache__":      true,
	".cache":           true,
	"cache":            true,
	"dist":             true,
	"build":            true,
	".next":            true,
	".nuxt":            true,
	".turbo":           true,
	".yarn":            true,
	".pnp":             true,
	"bower_components": true,
	"target":           true, // Rust
	".gradle":          true, // Java/Gradle
	".tox":             true, // Python tox
	".eggs":            true, // Python eggs
	" eggs":            true, // Python eggs (с пробелом)
	"Library/Caches":   true, // macOS
}

// WalkWithExclusions работает как filepath.Walk, но пропускает директории из SkipDirs.
// Если skipFn возвращает true для директории, она пропускается.
// skipFn может быть nil, тогда используется стандартный список SkipDirs.
func WalkWithExclusions(root string, fn filepath.WalkFunc, skipFn func(path string, info os.FileInfo) bool) error {
	if skipFn == nil {
		skipFn = func(path string, info os.FileInfo) bool {
			if !info.IsDir() {
				return false
			}
			return SkipDirs[filepath.Base(path)]
		}
	}

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // игнорируем ошибки доступа
		}

		// Сначала вызываем fn для текущего элемента (файла или директории)
		if err := fn(path, info, err); err != nil {
			return err
		}

		// Если это директория из списка исключений — не заходим внутрь,
		// но саму директорию уже обработали через fn выше
		if info.IsDir() && path != root {
			if skipFn(path, info) {
				return filepath.SkipDir
			}
		}

		return nil
	})
}

// FileExists проверяет, существует ли файл или директория по указанному пути.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Unique возвращает срез уникальных строк в порядке первого появления.
func Unique(items []string) []string {
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

// RunCommand запускает команду с аргументами и перенаправляет stdout/stderr в os.Stdout/os.Stderr.
// Возвращает ошибку выполнения команды.
func RunCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunCommandWithOutput запускает команду и возвращает её вывод (stdout) в виде строки.
func RunCommandWithOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("command %s %v failed: %v", name, args, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// FindGoMainOptions параметры для поиска main-файлов Go.
type FindGoMainOptions struct {
	SearchInCmdFirst bool     // сначала искать в cmd/
	ExcludeDirs      []string // директории для исключения (например, "vendor/", "internal/", ".git/")
	OnlyMainGo       bool     // искать только файлы с именем main.go (иначе все .go файлы)
}

// FindGoMain ищет Go файлы, объявляющие пакет main с функцией main.
// Возвращает пути к найденным файлам относительно корня проекта.
func FindGoMain(root string, opts FindGoMainOptions) ([]string, error) {
	var mains []string

	// Создаём skipFn, учитывающий opts.ExcludeDirs
	skipFn := func(path string, info os.FileInfo) bool {
		if !info.IsDir() {
			return false
		}
		// Проверка стандартных исключений
		if SkipDirs[filepath.Base(path)] {
			return true
		}
		// Проверка пользовательских исключений
		rel, _ := filepath.Rel(root, path)
		for _, excl := range opts.ExcludeDirs {
			if strings.Contains(rel, excl) {
				return true
			}
		}
		return false
	}

	// collect добавляет файл в результат, если он объявляет точку входа.
	collect := func(path string, info os.FileInfo) {
		if !isGoMainFile(info.Name(), path) {
			return
		}
		rel, _ := filepath.Rel(root, path)
		mains = append(mains, rel)
	}

	// Шаг 1: поиск в cmd/ если включено
	if opts.SearchInCmdFirst {
		cmdDir := filepath.Join(root, "cmd")
		if FileExists(cmdDir) {
			err := WalkWithExclusions(cmdDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				collect(path, info)
				return nil
			}, skipFn)
			if err != nil {
				return nil, err
			}
		}
		if len(mains) > 0 {
			return mains, nil
		}
	}

	// Шаг 2: поиск по всему проекту
	err := WalkWithExclusions(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Фильтр по имени файла
		if opts.OnlyMainGo && info.Name() != "main.go" {
			return nil
		}
		collect(path, info)
		return nil
	}, skipFn)
	if err != nil {
		return nil, err
	}
	return mains, nil
}

// isGoMainFile сообщает, объявляет ли файл пакет main с функцией main.
// Тестовые файлы пропускаются: они не являются точкой входа приложения.
func isGoMainFile(name, path string) bool {
	if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
		return false
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil || file.Name == nil || file.Name.Name != "main" {
		return false
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil {
			continue
		}
		if fn.Name.Name == "main" {
			return true
		}
	}
	return false
}

// FindGoMainPackages ищет пакеты main в проекте и возвращает директории этих
// пакетов без дублей. Это единая точка поиска для сборки и запуска.
func FindGoMainPackages(root string) ([]string, error) {
	mainFiles, err := FindGoMain(root, FindGoMainOptions{
		SearchInCmdFirst: true,
		ExcludeDirs:      []string{"vendor/", ".git/"},
		OnlyMainGo:       false,
	})
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(mainFiles))
	for _, f := range mainFiles {
		dirs = append(dirs, filepath.Dir(f))
	}
	return Unique(dirs), nil
}

// GoPackageArg приводит директорию пакета к аргументу go build/go run:
// относительный путь без префикса "." получает "./", иначе go трактует
// его как import path.
func GoPackageArg(dir string) string {
	if filepath.IsAbs(dir) || dir == "." ||
		strings.HasPrefix(dir, "./") || strings.HasPrefix(dir, "../") {
		return dir
	}
	return "./" + dir
}

// ResolveGoPackageTarget приводит явно указанную цель к аргументу
// go build/go run. Существующий .go-файл заменяется директорией пакета:
// при указании одного файла go не подхватывает остальные файлы пакета.
// Несуществующий путь считается import path и передаётся как есть.
func ResolveGoPackageTarget(target string) (string, error) {
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return target, nil
		}
		return "", fmt.Errorf(i18n.T("error accessing %s: %v"), target, err)
	}
	dir := target
	if !info.IsDir() {
		if !strings.HasSuffix(target, ".go") {
			return "", fmt.Errorf(i18n.T("target must be a .go file or a package directory: %s"), target)
		}
		dir = filepath.Dir(target)
	}
	return GoPackageArg(dir), nil
}

// SelectGoTarget возвращает цель сборки/запуска Go: явно указанную
// пользователем либо выбранную из найденных пакетов main.
// promptMsgid — ключ i18n подсказки выбора номера пакета.
func SelectGoTarget(explicit, promptMsgid string) (string, error) {
	if explicit != "" {
		return ResolveGoPackageTarget(explicit)
	}
	dirs, err := FindGoMainPackages(".")
	if err != nil {
		return "", fmt.Errorf(i18n.T("error finding main files: %v"), err)
	}
	if len(dirs) == 0 {
		return "", errors.New(i18n.T("no Go main package found"))
	}
	if len(dirs) == 1 {
		return GoPackageArg(dirs[0]), nil
	}
	dir, err := promptGoTarget(dirs, promptMsgid)
	if err != nil {
		return "", err
	}
	return GoPackageArg(dir), nil
}

// promptGoTarget предлагает пользователю выбрать директорию пакета по номеру.
func promptGoTarget(dirs []string, promptMsgid string) (string, error) {
	i18n.Printf("Multiple main packages found:\n")
	for i, d := range dirs {
		fmt.Printf("  %d) %s\n", i+1, d)
	}
	i18n.Printf(promptMsgid)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return dirs[0], nil
	}
	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(dirs) {
		return "", errors.New(i18n.T("invalid selection"))
	}
	return dirs[idx-1], nil
}
