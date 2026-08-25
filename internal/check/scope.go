package check

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Scope описывает объём кода, который будет проверяться.
type Scope struct {
	Name        string   // название объёма (для вывода)
	Files       []string // список файлов для проверки; пусто = весь проект
	Dirs        []string // уникальные директории изменённых файлов
	Changes     string   // изменения текстом
	FileChanges string   // полное содержимое измененных файлов
	kind        scopeKind
}

// Возвращает текст, который следует отправить на ревью.
// Для изменённых файлов — полный код файлов, иначе — текст изменений.
func (s Scope) GetChanges() string {
	if s.kind == scopeFiles {
		return s.FileChanges
	}
	return s.Changes
}

// FilesWithExt возвращает файлы Scope с одним из переданных расширений.
func (s Scope) FilesWithExt(exts ...string) []string {
	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}
	var result []string
	for _, f := range s.Files {
		if extSet[strings.ToLower(filepath.Ext(f))] {
			result = append(result, f)
		}
	}
	return result
}

// scopeKind — тип объёма проверки.
type scopeKind int

const (
	scopeChanged scopeKind = iota // изменённый код (незакоммиченные изменения)
	scopeFiles                    // измененные файлы (те, в которых есть незакомиченные изменения)
	scopeCommit1                  // изменённый код + 1 последний коммит
	scopeCommit2                  // + 2 коммита
	scopeCommit3                  // + 3 коммита
	scopeAll                      // весь код
	scopeMaster                   // разница текущей ветки с master
	scopeDevelop                  // разница текущей ветки с develop
)

// scopeOption — описание одного варианта выбора для пользователя.
type scopeOption struct {
	kind  scopeKind
	label string
}

// scopeOptions возвращает список вариантов выбора объёма проверки.
func scopeOptions() []scopeOption {
	return []scopeOption{
		{scopeChanged, "Changed code"},
		{scopeCommit1, "Changed code + 1 commit"},
		{scopeCommit2, "Changed code + 2 commits"},
		{scopeCommit3, "Changed code + 3 commits"},
		{scopeAll, "All code"},
		{scopeMaster, "Diff with master"},
		{scopeDevelop, "Diff with develop"},
	}
}

// scopeOptionsForAI возвращает варианты объёма для AI-ревью.
func scopeOptionsForAI() []scopeOption {
	return []scopeOption{
		{scopeChanged, "Changed code"},
		{scopeFiles, "Changed files"},
		{scopeCommit1, "Changed code + 1 commit"},
		{scopeCommit2, "Changed code + 2 commits"},
		{scopeCommit3, "Changed code + 3 commits"},
		{scopeMaster, "Diff with master"},
		{scopeDevelop, "Diff with develop"},
	}
}

// defaultScopeKind выбирает объём по умолчанию:
// изменённый код, если есть незакоммиченные изменения, иначе весь код.
func defaultScopeKind() scopeKind {
	if hasGit() {
		if len(changedFiles()) > 0 {
			return scopeChanged
		}
	}
	return scopeAll
}

// buildScope формирует Scope для заданного вида. Файлы из папок вендоров
// исключаются как из списка файлов, так и из текста diff.
func buildScope(kind scopeKind) Scope {
	switch kind {
	case scopeChanged:
		files := changedFiles()
		return makeScope(kind, "changed code", files, changedDiffText(files))
	case scopeFiles:
		files := changedFiles()
		return makeScope(kind, "changed files", files, changedDiffText(files))
	case scopeCommit1:
		files := filesSinceCommit(1)
		return makeScope(kind, "changed code + 1 commit", files, diffSinceCommitText(1, files))
	case scopeCommit2:
		files := filesSinceCommit(2)
		return makeScope(kind, "changed code + 2 commits", files, diffSinceCommitText(2, files))
	case scopeCommit3:
		files := filesSinceCommit(3)
		return makeScope(kind, "changed code + 3 commits", files, diffSinceCommitText(3, files))
	case scopeMaster:
		files := diffWithBranch("master")
		return makeScope(kind, "diff with master", files, diffBranchText("master", files))
	case scopeDevelop:
		files := diffWithBranch("develop")
		return makeScope(kind, "diff with develop", files, diffBranchText("develop", files))
	default:
		files := projectFiles()
		return Scope{
			kind:  scopeAll,
			Name:  "all code",
			Files: files,
			Dirs:  uniqueDirs(files),
		}
	}
}

// makeScope собирает Scope из файлов и текста изменений, вычисляя
// уникальные директории изменённых файлов.
func makeScope(kind scopeKind, name string, files []string, changes string) Scope {
	return Scope{
		kind:        kind,
		Name:        name,
		Files:       files,
		Dirs:        uniqueDirs(files),
		Changes:     changes,
		FileChanges: readScopeFiles(files),
	}
}

// uniqueDirs возвращает отсортированный список уникальных директорий
// для заданных файлов.
func uniqueDirs(files []string) []string {
	seen := make(map[string]bool)
	var dirs []string
	for _, f := range files {
		dir := filepath.Dir(f)
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)
	return dirs
}

// diffPathArgs формирует аргументы diff-команды для переданного списка путей.
// Если список пуст, возвращается nil (diff по всему проекту не строится).
func diffPathArgs(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	args := make([]string, 0, len(paths)+1)
	args = append(args, "--")
	args = append(args, paths...)
	return args
}

// changedDiffText возвращает текст незакоммиченных изменений (unstaged + staged)
func changedDiffText(paths []string) string {
	pathArgs := diffPathArgs(paths)
	if pathArgs == nil {
		return ""
	}
	var sb strings.Builder
	if out, err := gitOutput(append([]string{"diff"}, pathArgs...)...); err == nil {
		sb.WriteString(out)
	}
	if out, err := gitOutput(append([]string{"diff", "--cached"}, pathArgs...)...); err == nil {
		sb.WriteString(out)
	}
	return sb.String()
}

// diffSinceCommitText возвращает текст изменений за последние n коммитов
// вместе с незакоммиченными изменениями, ограниченный переданными путями.
func diffSinceCommitText(n int, paths []string) string {
	pathArgs := diffPathArgs(paths)
	if pathArgs == nil {
		return ""
	}
	var sb strings.Builder
	if n > 0 {
		base := []string{"diff", fmt.Sprintf("HEAD~%d", n), "HEAD"}
		if out, err := gitOutput(append(base, pathArgs...)...); err == nil {
			sb.WriteString(out)
		}
	}
	sb.WriteString(changedDiffText(paths))
	return sb.String()
}

// diffBranchText возвращает текст изменений текущей ветки относительно branch,
// ограниченный переданными путями.
func diffBranchText(branch string, paths []string) string {
	pathArgs := diffPathArgs(paths)
	if pathArgs == nil {
		return ""
	}
	base := []string{"diff", "origin/" + branch + "...HEAD"}
	out, err := gitOutput(append(base, pathArgs...)...)
	if err != nil {
		return ""
	}
	return out
}

// ScopeAll возвращает объём "весь код".
func ScopeAll() Scope {
	return buildScope(scopeAll)
}

// ScopeChanged возвращает объём "изменённый код".
func ScopeChanged() Scope {
	return buildScope(scopeChanged)
}

// ScopeCommits возвращает объём "изменённый код + N последних коммитов".
func ScopeCommits(n int) Scope {
	if n <= 0 {
		n = 1
	}
	return buildScope(scopeKindFromCommits(n))
}

// scopeKindFromCommits возвращает вид объёма для N коммитов.
func scopeKindFromCommits(n int) scopeKind {
	switch n {
	case 1:
		return scopeCommit1
	case 2:
		return scopeCommit2
	case 3:
		return scopeCommit3
	default:
		return scopeCommit1
	}
}

// ScopeDiff возвращает объём "разница текущей ветки с branch".
func ScopeDiff(branch string) Scope {
	switch branch {
	case "master":
		return buildScope(scopeMaster)
	case "develop":
		return buildScope(scopeDevelop)
	default:
		return buildScope(scopeAll)
	}
}

// ScopeDefault возвращает объём по умолчанию: изменённый код, если есть
// незакоммиченные изменения, иначе весь код.
func ScopeDefault() Scope {
	return buildScope(defaultScopeKind())
}

// hasGit проверяет, что текущая директория находится в git-репозитории.
func hasGit() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// changedFiles возвращает список изменённых файлов:
// незакоммиченные (unstaged), staged и untracked.
func changedFiles() []string {
	var result []string
	// Изменённые и удалённые в рабочей директории.
	if out, err := gitOutput("diff", "--name-only"); err == nil {
		result = append(result, lines(out)...)
	}
	// Staged.
	if out, err := gitOutput("diff", "--cached", "--name-only"); err == nil {
		result = append(result, lines(out)...)
	}
	// Untracked.
	if out, err := gitOutput("ls-files", "--others", "--exclude-standard"); err == nil {
		result = append(result, lines(out)...)
	}
	return uniqueSorted(filterVendor(result))
}

// filesSinceCommit возвращает изменённые файлы за последние n коммитов
// вместе с незакоммиченными изменениями.
func filesSinceCommit(n int) []string {
	var result []string
	if n > 0 {
		// Файлы, затронутые в последних n коммитах.
		if out, err := gitOutput("diff", fmt.Sprintf("HEAD~%d", n), "HEAD", "--name-only"); err == nil {
			result = append(result, lines(out)...)
		}
	}
	result = append(result, changedFiles()...)
	return uniqueSorted(result)
}

// diffWithBranch возвращает файлы, изменённые относительно ветки branch.
func diffWithBranch(branch string) []string {
	var result []string
	if out, err := gitOutput("diff", "origin/"+branch+"...HEAD", "--name-only"); err == nil {
		result = append(result, lines(out)...)
	}
	return uniqueSorted(filterVendor(result))
}

// vendorDirNames — имена каталогов вендоров, которые не должны попадать
// на проверку: это внешний код, который не относится к разработке проекта.
var vendorDirNames = map[string]bool{
	"vendor":           true, // Go, PHP Composer и др.
	"node_modules":     true, // npm/yarn/pnpm
	"venv":             true, // Python virtualenv
	".venv":            true, // Python virtualenv (скрытый вариант)
	"bower_components": true, // Bower
	"third_party":      true, // третья сторона
	"Pods":             true, // CocoaPods
	"__pycache__":      true, // Python кэш байт-кода
	"site-packages":    true, // Python установленные пакеты
	"dist":             true, // сборки (иногда содержат вендорный код)
}

// isVendorPath определяет, находится ли путь внутри каталога вендора.
// Проверяются все сегменты пути, поэтому вложенные папки вендоров также
// отфильтровываются. Пути нормализуются с прямыми слешами.
func isVendorPath(path string) bool {
	p := filepath.ToSlash(path)
	for _, seg := range strings.Split(p, "/") {
		if vendorDirNames[seg] {
			return true
		}
	}
	return false
}

// filterVendor оставляет только файлы, не находящиеся в каталогах вендоров.
func filterVendor(files []string) []string {
	var result []string
	for _, f := range files {
		if !isVendorPath(f) {
			result = append(result, f)
		}
	}
	return result
}

// projectFiles возвращает список всех файлов проекта (отслеживаемых и
// untracked), исключая каталоги вендоров через filterVendor. Используется
// для полной проверки (scopeAll), когда нужно проверить весь код, а не
// только изменения. Если git недоступен, возвращается пустой список.
func projectFiles() []string {
	var result []string
	if out, err := gitOutput("ls-files"); err == nil {
		result = append(result, lines(out)...)
	}
	if out, err := gitOutput("ls-files", "--others", "--exclude-standard"); err == nil {
		result = append(result, lines(out)...)
	}
	return uniqueSorted(filterVendor(result))
}

// commitSubjectLimit — максимальная длина отображаемого текста коммита.
const commitSubjectLimit = 30

// latestCommitNames возвращает темы (subject) последних n коммитов.
// Хэш коммита не показывается, текст ограничен commitSubjectLimit символами.
func latestCommitNames(n int) []string {
	// %s — только тема коммита, без хэша.
	out, err := gitOutput("log", "-n", fmt.Sprintf("%d", n), "--pretty=%s")
	if err != nil {
		return nil
	}
	names := lines(out)
	for i, name := range names {
		runes := []rune(name)
		if len(runes) > commitSubjectLimit {
			names[i] = string(runes[:commitSubjectLimit]) + "..."
		}
	}
	return names
}

// gitOutput выполняет git-команду и возвращает stdout без ошибки.
func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// lines разбивает строку на непустые строки.
func lines(s string) []string {
	var result []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			result = append(result, l)
		}
	}
	return result
}

// uniqueSorted возвращает уникальные строки в отсортированном порядке.
func uniqueSorted(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, it := range items {
		if !seen[it] {
			seen[it] = true
			result = append(result, it)
		}
	}
	sort.Strings(result)
	return result
}

// promptScope показывают пользователю список вариантов и ждёт выбора.
// Возвращает выбранный объём. При пустом вводе используется вариант по умолчанию.
func promptScope() Scope {
	return promptScopeWithOptions(scopeOptions(), defaultScopeKind())
}

// promptScopeForAI показывает пользователю варианты объёма для AI-ревью.
// По умолчанию выбран вариант "Changed code".
func promptScopeForAI() Scope {
	return promptScopeWithOptions(scopeOptionsForAI(), scopeChanged)
}

// promptScopeWithOptions показывает список вариантов выбора и ждёт ответа.
// При пустом вводе используется defaultKind.
func promptScopeWithOptions(options []scopeOption, defaultKind scopeKind) Scope {
	fmt.Println("\nSelect scope:")
	for i, opt := range options {
		fmt.Printf("  %d) %s", i+1, opt.label)
		// Для вариантов с N коммитами показываем их конкретные названия.
		switch opt.kind {
		case scopeCommit1, scopeCommit2, scopeCommit3:
			n := 1
			if opt.kind == scopeCommit2 {
				n = 2
			} else if opt.kind == scopeCommit3 {
				n = 3
			}
			names := latestCommitNames(n)
			if len(names) > 0 {
				fmt.Printf("  (%s)", strings.Join(names, ", "))
			}
		}
		// Показываем размер отправляемого текста в кратком формате (напр. 44k)
		if hint := scopeSizeHint(opt.kind); hint != "" {
			fmt.Printf("  (%s)", hint)
		}
		fmt.Println()
	}
	// Подсказка по умолчанию.
	var defaultIdx int
	for i, opt := range options {
		if opt.kind == defaultKind {
			defaultIdx = i
			break
		}
	}

	var input string
	fmt.Printf("\nSelect [%d]: ", defaultIdx+1)
	fmt.Fscanln(os.Stdin, &input)
	input = strings.TrimSpace(input)

	idx := defaultIdx
	if input != "" {
		if n, err := parseInt(input); err == nil && n >= 1 && n <= len(options) {
			idx = n - 1
		}
	}
	return buildScope(options[idx].kind)
}

// parseInt разбирает число из строки.
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// scopeSizeHint возвращает краткое представление размера текста, который
// будет отправлен на ревью для заданного вида объёма. Возвращает пустую
// строку, если размер неинформативен (нулевой или текста нет вовсе), —
// тогда в меню размер не выводится.
func scopeSizeHint(kind scopeKind) string {
	if kind == scopeAll {
		return ""
	}
	n := len(buildScope(kind).GetChanges())
	if n <= 0 {
		return ""
	}
	return formatSize(n)
}

// formatSize форматирует количество символов в краткий человекочитаемый вид:
// 44k, 1.2m и т.п. Для значений меньше тысячи — просто число.
func formatSize(n int) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.1fm", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
