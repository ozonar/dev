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

// buildScope формирует Scope для заданного вида.
func buildScope(kind scopeKind) Scope {
	switch kind {
	case scopeChanged:
		return makeScope(kind, "changed code", changedFiles(), changedDiffText())
	case scopeFiles:
		return makeScope(kind, "changed files", changedFiles(), changedDiffText())
	case scopeCommit1:
		return makeScope(kind, "changed code + 1 commit", filesSinceCommit(1), diffSinceCommitText(1))
	case scopeCommit2:
		return makeScope(kind, "changed code + 2 commits", filesSinceCommit(2), diffSinceCommitText(2))
	case scopeCommit3:
		return makeScope(kind, "changed code + 3 commits", filesSinceCommit(3), diffSinceCommitText(3))
	case scopeMaster:
		return makeScope(kind, "diff with master", diffWithBranch("master"), diffBranchText("master"))
	case scopeDevelop:
		return makeScope(kind, "diff with develop", diffWithBranch("develop"), diffBranchText("develop"))
	default:
		return Scope{Name: "all code"}
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

// changedDiffText возвращает текст незакоммиченных изменений (unstaged + staged).
func changedDiffText() string {
	var sb strings.Builder
	if out, err := gitOutput("diff"); err == nil {
		sb.WriteString(out)
	}
	if out, err := gitOutput("diff", "--cached"); err == nil {
		sb.WriteString(out)
	}
	return sb.String()
}

// diffSinceCommitText возвращает текст изменений за последние n коммитов
// вместе с незакоммиченными изменениями.
func diffSinceCommitText(n int) string {
	var sb strings.Builder
	if n > 0 {
		if out, err := gitOutput("diff", fmt.Sprintf("HEAD~%d", n), "HEAD"); err == nil {
			sb.WriteString(out)
		}
	}
	sb.WriteString(changedDiffText())
	return sb.String()
}

// diffBranchText возвращает текст изменений текущей ветки относительно branch.
func diffBranchText(branch string) string {
	out, err := gitOutput("diff", "origin/"+branch+"...HEAD")
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
	return uniqueSorted(result)
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
	return uniqueSorted(result)
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
