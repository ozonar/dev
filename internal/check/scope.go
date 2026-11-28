package check

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"dev/internal/i18n"
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
	scopeBranch                   // разница текущей ветки с базовой (branch)
)

// scopeOption — описание одного варианта выбора для пользователя.
type scopeOption struct {
	kind   scopeKind
	branch string // базовая ветка для scopeBranch
	label  string
}

// baseBranchCandidates — ветки, с которыми имеет смысл сравнивать текущую.
// В меню попадают только те из них, что реально существуют в репозитории.
var baseBranchCandidates = []string{"develop", "main", "master"}

// branchOptions возвращает варианты "Diff with <branch>" для существующих
// базовых веток. Вне git или при отсутствии веток список пуст.
func branchOptions() []scopeOption {
	var options []scopeOption
	for _, branch := range baseBranchCandidates {
		if _, ok := branchRef(branch); !ok {
			continue
		}
		options = append(options, scopeOption{
			kind:   scopeBranch,
			branch: branch,
			label:  i18n.T("Diff with %s", branch),
		})
	}
	return options
}

// scopeOptions возвращает список вариантов выбора объёма проверки.
func scopeOptions() []scopeOption {
	options := []scopeOption{
		{kind: scopeChanged, label: i18n.T("Changed code")},
		{kind: scopeCommit1, label: i18n.T("Changed code + 1 commit")},
		{kind: scopeCommit2, label: i18n.T("Changed code + 2 commits")},
		{kind: scopeCommit3, label: i18n.T("Changed code + 3 commits")},
		{kind: scopeAll, label: i18n.T("All code")},
	}
	return append(options, branchOptions()...)
}

// scopeOptionsForAI возвращает варианты объёма для AI-ревью.
func scopeOptionsForAI() []scopeOption {
	options := []scopeOption{
		{kind: scopeChanged, label: i18n.T("Changed code")},
		{kind: scopeFiles, label: i18n.T("Changed files")},
		{kind: scopeCommit1, label: i18n.T("Changed code + 1 commit")},
		{kind: scopeCommit2, label: i18n.T("Changed code + 2 commits")},
		{kind: scopeCommit3, label: i18n.T("Changed code + 3 commits")},
	}
	return append(options, branchOptions()...)
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

// buildScope формирует Scope по выбранному варианту. Файлы из папок вендоров
// исключаются как из списка файлов, так и из текста diff.
// Вне git история изменений недоступна, поэтому любой вид без базовой ветки
// означает весь код: иначе проверять было бы нечего.
func buildScope(opt scopeOption) Scope {
	if opt.kind == scopeBranch {
		return buildBranchScope(opt.branch)
	}
	kind := opt.kind
	if !hasGit() {
		kind = scopeAll
	}
	switch kind {
	case scopeChanged:
		files := changedFiles()
		return makeScope(kind, i18n.T("changed code"), files, changedDiffText(files))
	case scopeFiles:
		files := changedFiles()
		return makeScope(kind, i18n.T("changed files"), files, changedDiffText(files))
	case scopeCommit1:
		files := filesSinceCommit(1)
		return makeScope(kind, i18n.T("changed code + 1 commit"), files, diffSinceCommitText(1, files))
	case scopeCommit2:
		files := filesSinceCommit(2)
		return makeScope(kind, i18n.T("changed code + 2 commits"), files, diffSinceCommitText(2, files))
	case scopeCommit3:
		files := filesSinceCommit(3)
		return makeScope(kind, i18n.T("changed code + 3 commits"), files, diffSinceCommitText(3, files))
	default:
		files := existingFiles(projectFiles())
		return Scope{
			kind:  scopeAll,
			Name:  i18n.T("all code"),
			Files: files,
			Dirs:  uniqueDirs(files),
		}
	}
}

// buildBranchScope формирует объём "разница текущей ветки с branch".
func buildBranchScope(branch string) Scope {
	files := diffWithBranch(branch)
	return makeScope(scopeBranch, i18n.T("Diff with %s", branch), files, diffBranchText(branch, files))
}

// makeScope собирает Scope из файлов и текста изменений, вычисляя
// уникальные директории изменённых файлов. Удалённые из рабочей копии
// файлы (git diff возвращает их при удалении) исключаются из списка Files:
// проверять их линтерами нельзя, а текст изменений (changes) приходит уже
// сформированным с учётом удалений и остаётся нетронутым.
func makeScope(kind scopeKind, name string, files []string, changes string) Scope {
	existing := existingFiles(files)

	var fileChanges string
	if kind == scopeFiles {
		fileChanges = readScopeFiles(existing)
	}
	return Scope{
		kind:        kind,
		Name:        name,
		Files:       existing,
		Dirs:        uniqueDirs(existing),
		Changes:     changes,
		FileChanges: fileChanges,
	}
}

// existingFiles оставляет только файлы, которые реально существуют на диске
// (и не являются директориями). Остальные отбрасываются.
func existingFiles(files []string) []string {
	var result []string
	for _, f := range files {
		if info, err := os.Stat(f); err == nil && !info.IsDir() {
			result = append(result, f)
		}
	}
	return result
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
// и содержимое новых файлов: обычный diff untracked-файлы не показывает.
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
	sb.WriteString(untrackedFilesText(paths))
	return sb.String()
}

// untrackedFilesText возвращает новые файлы в том же diff-формате, что и
// staged-изменения: иначе размер ревью зависел бы от наличия git add.
func untrackedFilesText(paths []string) string {
	untracked := untrackedSet()
	var sb strings.Builder
	for _, path := range paths {
		if !untracked[path] {
			continue
		}
		lines, ok := readFileLines(path)
		if !ok {
			continue
		}
		sb.WriteString(untrackedFileDiff(filepath.ToSlash(path), trimTrailingEmpty(lines)))
	}
	return sb.String()
}

// untrackedFileDiff формирует diff нового файла в формате git.
func untrackedFileDiff(path string, lines []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", path, path)
	sb.WriteString("new file mode 100644\n")
	sb.WriteString("--- /dev/null\n")
	fmt.Fprintf(&sb, "+++ b/%s\n", path)
	if len(lines) == 0 {
		return sb.String()
	}
	fmt.Fprintf(&sb, "@@ -0,0 +1,%d @@\n", len(lines))
	for _, line := range lines {
		sb.WriteString("+")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String()
}

// trimTrailingEmpty убирает завершающую пустую строку после Split.
func trimTrailingEmpty(lines []string) []string {
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	return lines
}

// untrackedSet возвращает множество новых файлов, не отслеживаемых git.
func untrackedSet() map[string]bool {
	result := make(map[string]bool)
	if out, err := gitOutput("ls-files", "--others", "--exclude-standard"); err == nil {
		for _, f := range lines(out) {
			result[f] = true
		}
	}
	return result
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

// branchRef возвращает ссылку для сравнения с веткой: origin/<branch>,
// если она есть, иначе локальную <branch>. ok=false — ветки нет.
func branchRef(branch string) (string, bool) {
	if branch == "" {
		return "", false
	}
	if _, err := gitOutput("rev-parse", "--verify", "--quiet", "origin/"+branch); err == nil {
		return "origin/" + branch, true
	}
	if _, err := gitOutput("rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		return branch, true
	}
	return "", false
}

// diffBranchText возвращает текст изменений текущей ветки относительно branch,
// ограниченный переданными путями.
func diffBranchText(branch string, paths []string) string {
	pathArgs := diffPathArgs(paths)
	if pathArgs == nil {
		return ""
	}
	ref, ok := branchRef(branch)
	if !ok {
		return ""
	}
	out, err := gitOutput(append([]string{"diff", ref + "...HEAD"}, pathArgs...)...)
	if err != nil {
		return ""
	}
	return out
}

// ScopeAll возвращает объём "весь код".
func ScopeAll() Scope {
	return buildScope(scopeOption{kind: scopeAll})
}

// ScopeChanged возвращает объём "изменённый код".
func ScopeChanged() Scope {
	return buildScope(scopeOption{kind: scopeChanged})
}

// ScopeCommits возвращает объём "изменённый код + N последних коммитов".
func ScopeCommits(n int) Scope {
	if n <= 0 {
		n = 1
	}
	return buildScope(scopeOption{kind: scopeKindFromCommits(n)})
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
// Несуществующая ветка — ошибка: иначе на проверку попал бы не тот код.
func ScopeDiff(branch string) (Scope, error) {
	if _, ok := branchRef(branch); !ok {
		return Scope{}, fmt.Errorf("branch %q not found", branch)
	}
	return buildScope(scopeOption{kind: scopeBranch, branch: branch}), nil
}

// ScopeDefault возвращает объём по умолчанию: изменённый код, если есть
// незакоммиченные изменения, иначе весь код.
func ScopeDefault() Scope {
	return buildScope(scopeOption{kind: defaultScopeKind()})
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
	ref, ok := branchRef(branch)
	if !ok {
		return nil
	}
	out, err := gitOutput("diff", ref+"...HEAD", "--name-only")
	if err != nil {
		return nil
	}
	return uniqueSorted(filterVendor(lines(out)))
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
// только изменения. Вне git список собирается обходом текущей директории
func projectFiles() []string {
	if !hasGit() {
		return walkProjectFiles(".")
	}
	var result []string
	if out, err := gitOutput("ls-files"); err == nil {
		result = append(result, lines(out)...)
	}
	if out, err := gitOutput("ls-files", "--others", "--exclude-standard"); err == nil {
		result = append(result, lines(out)...)
	}
	return uniqueSorted(filterVendor(result))
}

// walkProjectFiles собирает файлы проекта обходом файловой системы от root.
// Вендорские и служебные каталоги (VCS, IDE) пропускаются целиком: их файлы
// не относятся к коду проекта. Недоступные элементы пропускаются, чтобы одна
// ошибка доступа не прерывала обход.
func walkProjectFiles(root string) []string {
	// Каталоги, обход которых не нужен. Вендоры исключаются по тем же именам,
	// что и в filterVendor, чтобы обход не зависел от отдельного списка.
	skip := map[string]bool{
		".git": true, ".hg": true, ".svn": true, ".idea": true, ".vscode": true,
	}
	for name := range vendorDirNames {
		skip[name] = true
	}

	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skip[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		files = append(files, path)
		return nil
	})
	return uniqueSorted(filterVendor(files))
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
// Для статической проверки показывается количество файлов в объёме.
func promptScope() Scope {
	return promptScopeWithOptions(scopeOptions(), defaultScopeKind(), scopeFileHint)
}

// promptScopeForAI показывает пользователю варианты объёма для AI-ревью.
// По умолчанию выбран вариант "Changed code".
// Для AI-ревью показывается размер отправляемого текста (в символах).
// При expand подсказка показывает, до какого бюджета текст может вырасти.
func promptScopeForAI(expand bool, budget int) Scope {
	return promptScopeWithOptions(scopeOptionsForAI(), scopeChanged, scopeSizeHintForAI(expand, budget))
}

// promptScopeWithOptions показывает список вариантов выбора и ждёт ответа.
// При пустом вводе используется defaultKind. hint формирует подсказку
// о составе объёма (количество файлов для линтеров или символов для AI).
func promptScopeWithOptions(options []scopeOption, defaultKind scopeKind, hint func(scopeOption) string) Scope {
	fmt.Println()
	i18n.Printf("Select scope:\n")
	for i, opt := range options {
		fmt.Printf("  %d) %s", i+1, opt.label)
		// Для вариантов с N коммитами показываем их конкретные названия.
		switch opt.kind {
		case scopeCommit1, scopeCommit2, scopeCommit3:
			n := 1
			switch opt.kind {
			case scopeCommit2:
				n = 2
			case scopeCommit3:
				n = 3
			}
			names := latestCommitNames(n)
			if len(names) > 0 {
				fmt.Printf("  (%s)", strings.Join(names, ", "))
			}
		}
		if h := hint(opt); h != "" {
			fmt.Printf("  (%s)", h)
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
	fmt.Println()
	i18n.Printf("Select [%d]: ", defaultIdx+1)
	if _, err := fmt.Fscanln(os.Stdin, &input); err != nil {
		input = ""
	}
	input = strings.TrimSpace(input)

	idx := defaultIdx
	if input != "" {
		if n, err := parseInt(input); err == nil && n >= 1 && n <= len(options) {
			idx = n - 1
		}
	}
	return buildScope(options[idx])
}

// parseInt разбирает число из строки.
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// scopeText возвращает текст, который будет отправлен на ревью для заданного
// вида объёма. ok=false означает, что текста нет.
func scopeText(opt scopeOption) (string, bool) {
	if opt.kind == scopeAll {
		return "", false
	}
	scope := buildScope(opt)
	if scope.kind == scopeAll {
		return "", false
	}
	text := scope.GetChanges()
	if text == "" {
		return "", false
	}
	return text, true
}

// scopeSizeHint возвращает краткое представление размера текста, который
// будет отправлен на ревью для заданного вида объёма. Возвращает пустую
// строку, если размер неинформативен (нулевой или текста нет вовсе), —
// тогда в меню размер не выводится.
func scopeSizeHint(opt scopeOption) string {
	text, ok := scopeText(opt)
	if !ok {
		return ""
	}
	return formatSize(len([]rune(text)))
}

// scopeSizeHintForAI возвращает подсказку о размере текста для AI-ревью.
// При расширении контекста показывает фактический размер после добавления
// целых функций вокруг изменённых строк.
func scopeSizeHintForAI(expand bool, budget int) func(scopeOption) string {
	return func(opt scopeOption) string {
		text, ok := scopeText(opt)
		if !ok {
			return ""
		}
		size := len([]rune(text))
		if expand {
			expanded, added := expandChangesToFunctions(text, budget)
			if added > 0 {
				size = len([]rune(expanded))
			}
		}
		return formatSize(size)
	}
}

// scopeFileHint возвращает количество файлов в объёме, например "3 files".
// Возвращает пустую строку, если файлов нет вовсе или объём — весь код.
// Используется для статической проверки, где важен объём анализа.
func scopeFileHint(opt scopeOption) string {
	if opt.kind == scopeAll {
		return ""
	}
	scope := buildScope(opt)
	if scope.kind == scopeAll {
		return ""
	}
	n := len(scope.Files)
	if n <= 0 {
		return ""
	}
	return i18n.N("%d files", n)
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
