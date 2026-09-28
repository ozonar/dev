package check

import (
	"os"
	"path/filepath"
	"sort"
)

// goLintRun описывает один запуск golangci-lint: рабочая директория —
// корень Go-модуля (содержит go.mod) — и пути анализа относительно неё.
// golangci-lint принимает пути только внутри своего модуля, поэтому каждая
// группа путей жёстко привязана к своему корню: запуск вне модуля падает
// с ошибкой "no go files to analyze".
type goLintRun struct {
	Dir   string   // корень Go-модуля (абсолютный путь)
	Paths []string // пути анализа относительно Dir
}

// findGoModuleRoot возвращает абсолютный путь к корню Go-модуля, содержащего
// директорию dir: ближайший go.mod, найденный при подъёме вверх по дереву.
// ok=false, если go.mod не найден ни в dir, ни в родительских директориях.
func findGoModuleRoot(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for d := abs; ; d = filepath.Dir(d) {
		if info, err := os.Stat(filepath.Join(d, "go.mod")); err == nil && !info.IsDir() {
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
	}
}

// goLintRuns формирует запуски golangci-lint для объёма проверки.
// Для каждого найденного корня Go-модуля создаётся отдельный запуск:
// golangci-lint работает только внутри своего модуля, а разные модули
// имеют разные рабочие директории.
// Для полного объёма (scopeAll) каждый модуль анализируется целиком (./...),
// для изменённого кода — только затронутые директории, пересчитанные
// относительно корня модуля.
// Пустой результат означает, что Go-модуль для проверяемого кода не найден, —
// линтер запускать нельзя, иначе он упадёт с "no go files to analyze".
func goLintRuns(scope Scope) []goLintRun {
	if scope.kind == scopeAll {
		return allGoLintRuns(scope.Dirs)
	}
	return changedGoLintRuns(scope.Dirs)
}

// allGoLintRuns формирует по одному запуску ./... на каждый корень Go-модуля,
// встречающийся среди директорий проекта: полный анализ не должен зависеть
// от того, какие именно директории попали в список.
func allGoLintRuns(dirs []string) []goLintRun {
	var runs []goLintRun
	for _, root := range moduleRoots(dirs) {
		runs = append(runs, goLintRun{Dir: root, Paths: []string{"./..."}})
	}
	return runs
}

// changedGoLintRuns группирует директории с Go-файлами по корням их модулей
// и пересчитывает пути относительно корня — golangci-lint принимает пути
// только внутри модуля, из которого запущен.
func changedGoLintRuns(dirs []string) []goLintRun {
	// Оставляем только директории с Go-файлами: каталоги без .go golangci-lint
	// отвергает той же ошибкой "no go files to analyze".
	goDirs := goDirArgs(dirs)
	if len(goDirs) == 0 {
		return nil
	}

	byRoot := make(map[string][]string)
	var order []string
	for _, d := range goDirs {
		root, ok := findGoModuleRoot(d)
		if !ok {
			continue
		}
		// findGoModuleRoot возвращает абсолютный корень, а d — путь из scope
		// (относительный рабочей директории). Для вычисления относительного
		// пути нормализуем d в абсолютный, иначе filepath.Rel завершится
		// ошибкой из-за несовместимости относительного и абсолютного путей.
		absD, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, absD)
		if err != nil {
			continue
		}
		if _, seen := byRoot[root]; !seen {
			order = append(order, root)
		}
		byRoot[root] = append(byRoot[root], filepath.ToSlash(rel))
	}

	sort.Strings(order)
	runs := make([]goLintRun, 0, len(order))
	for _, root := range order {
		paths := byRoot[root]
		sort.Strings(paths)
		runs = append(runs, goLintRun{Dir: root, Paths: paths})
	}
	return runs
}

// moduleRoots возвращает отсортированный список уникальных корней Go-модулей
// для заданных директорий. Директории без модуля (go.mod не найден выше)
// пропускаются.
func moduleRoots(dirs []string) []string {
	seen := make(map[string]bool)
	var roots []string
	for _, d := range dirs {
		if root, ok := findGoModuleRoot(d); ok && !seen[root] {
			seen[root] = true
			roots = append(roots, root)
		}
	}
	sort.Strings(roots)
	return roots
}

// buildGoLintArgs формирует аргументы golangci-lint для одного запуска:
// run [--fix] <paths>.
func buildGoLintArgs(run goLintRun, mode Mode) []string {
	args := []string{"run"}
	if mode == ModeFix {
		args = append(args, "--fix")
	}
	args = append(args, run.Paths...)
	return args
}
