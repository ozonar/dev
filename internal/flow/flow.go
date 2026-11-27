package flow

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"dev/internal/common"
	"dev/internal/i18n"
)

const (
	// DefaultDays — период по умолчанию для показа последних веток.
	DefaultDays = 30
	// recentLimit — максимальное число веток в списке выбора.
	recentLimit = 20
)

// CopyChanges определяет, что переносить в новую ветку в режиме copy.
type CopyChanges int

const (
	// CopyUncommitted переносит только незакоммиченные изменения.
	CopyUncommitted CopyChanges = iota
	// CopyAll переносит коммиты текущей ветки и незакоммиченные изменения.
	CopyAll
)

// Options — параметры запуска сценария flow.
// Branch задан — сценарий работает без вопросов о ветке.
type Options struct {
	Branch string
	From   string
	Days   int
	In     io.Reader
}

// CopyOptions — параметры сценария flow copy.
type CopyOptions struct {
	Options
	Changes CopyChanges
}

// ParseCopyChanges разбирает значение флага --changes.
func ParseCopyChanges(s string) (CopyChanges, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "uncommitted":
		return CopyUncommitted, nil
	case "all":
		return CopyAll, nil
	default:
		return 0, fmt.Errorf("invalid --changes %q: use uncommitted or all", s)
	}
}

// Run переключает на существующую ветку либо создаёт новую, сохраняя
// незакоммиченную работу в стэш и возвращая ранее отложенную.
func Run(opts Options) error {
	opts = withDefaults(opts)
	if !isGitRepo() {
		return fmt.Errorf("%s", i18n.T("not a git repository"))
	}
	cur, err := currentBranch()
	if err != nil {
		return err
	}
	def, err := defaultBranch()
	if err != nil {
		return err
	}

	target := strings.TrimSpace(opts.Branch)
	interactive := target == ""
	fromList := false
	if interactive {
		locals, err := listLocalBranches()
		if err != nil {
			return err
		}
		recent := recentBranches(locals, def, time.Now().AddDate(0, 0, -opts.Days), recentLimit)
		target, fromList, err = promptTarget(recent, opts.Days, opts.In)
		if err != nil {
			return err
		}
		if target == "" {
			i18n.Yellow("Cancelled.")
			return nil
		}
	}

	if err := validateBranchName(target); err != nil {
		return err
	}
	if target == cur {
		i18n.Yellow("Already on branch %s.", target)
		return nil
	}
	remote, onRemote := remoteBranchRef(target)
	// Введённое вручную имя существующей ветки — это переключение на неё,
	// о чём нужно предупредить: пользователь мог ожидать создания новой.
	if interactive && !fromList && (localBranchExists(target) || onRemote) {
		if !confirm(opts.In, "Branch %s already exists, switching to it? [Y/n]: ", target) {
			i18n.Yellow("Cancelled.")
			return nil
		}
	}
	if localBranchExists(target) {
		return switchExisting(cur, target)
	}
	if onRemote {
		return switchTracking(cur, target, remote)
	}

	base, err := chooseBase(opts, def, interactive, true)
	if err != nil {
		return err
	}
	return createBranch(cur, target, base)
}

// RunCopy создаёт новую ветку и переносит в неё незакоммиченные изменения
// либо все уникальные изменения текущей ветки.
func RunCopy(opts CopyOptions) error {
	opts.Options = withDefaults(opts.Options)
	if !isGitRepo() {
		return fmt.Errorf("%s", i18n.T("not a git repository"))
	}
	cur, err := currentBranch()
	if err != nil {
		return err
	}
	def, err := defaultBranch()
	if err != nil {
		return err
	}

	target := strings.TrimSpace(opts.Branch)
	interactive := target == ""
	if interactive {
		locals, err := listLocalBranches()
		if err != nil {
			return err
		}
		recent := recentBranches(locals, def, time.Now().AddDate(0, 0, -opts.Days), recentLimit)
		printRecent(recent, opts.Days)
		target = promptLine(opts.In, "New branch name: ")
		if target == "" {
			i18n.Yellow("Cancelled.")
			return nil
		}
	}

	if err := validateBranchName(target); err != nil {
		return err
	}
	if localBranchExists(target) {
		return fmt.Errorf("%s", i18n.T("Branch %q already exists", target))
	}
	if _, ok := remoteBranchRef(target); ok {
		return fmt.Errorf("%s", i18n.T("Branch %q already exists", target))
	}

	base, err := chooseBase(opts.Options, def, interactive, false)
	if err != nil {
		return err
	}
	changes := opts.Changes
	if interactive {
		changes, err = promptChanges(opts.In)
		if err != nil {
			return err
		}
	}
	return copyToBranch(cur, target, base, changes)
}

// withDefaults заполняет значения, без которых сценарий не имеет смысла.
func withDefaults(opts Options) Options {
	if opts.Days <= 0 {
		opts.Days = DefaultDays
	}
	if opts.In == nil {
		opts.In = os.Stdin
	}
	return opts
}

// promptTarget показывает список веток и читает номер или имя.
// Пустой ввод означает отмену. Второй результат сообщает, что ветка выбрана
// из списка: тогда подтверждение переключения не требуется.
func promptTarget(recent []branch, days int, in io.Reader) (string, bool, error) {
	printRecent(recent, days)
	input := promptLine(in, "Branch name or number: ")
	if input == "" {
		return "", false, nil
	}
	if n, err := strconv.Atoi(input); err == nil {
		if n < 1 || n > len(recent) {
			return "", false, fmt.Errorf("%s", i18n.T("invalid selection"))
		}
		return recent[n-1].Name, true, nil
	}
	// Число вне диапазона int — некорректный выбор, а не имя ветки.
	if allDigits(input) {
		return "", false, fmt.Errorf("%s", i18n.T("invalid selection"))
	}
	return input, false, nil
}

// printRecent выводит недавние ветки с пометкой веток текущего пользователя.
func printRecent(recent []branch, days int) {
	if len(recent) == 0 {
		i18n.Yellow("No recent branches found.")
		return
	}
	i18n.Cyan("Recent branches (last %d days):", days)
	for i, b := range recent {
		when := humanSince(b.LastCommit)
		if b.Mine {
			when = i18n.T("mine") + ", " + when
		}
		fmt.Printf("  %d) %s  (%s)\n", i+1, b.Name, when)
	}
}

// chooseBase выбирает базовую ветку для новой ветки. Неинтерактивный режим
// использует --from либо значение по умолчанию без вопросов.
func chooseBase(opts Options, def string, interactive, preferDevelop bool) (string, error) {
	names, err := allBranchNames()
	if err != nil {
		return "", err
	}
	if from := strings.TrimSpace(opts.From); from != "" {
		if !containsName(names, from) {
			return "", fmt.Errorf("%s", i18n.T("base branch %q not found", from))
		}
		return from, nil
	}
	candidates := baseCandidates(names, def)
	if len(candidates) == 0 {
		return "", fmt.Errorf("%s", i18n.T("no base branch found"))
	}
	defBase := defaultBase(def, candidates, preferDevelop)
	if !interactive {
		return defBase, nil
	}

	fmt.Println()
	i18n.Printf("Select base branch:\n")
	for i, name := range candidates {
		marker := ""
		if name == defBase {
			marker = "  " + i18n.T("(default)")
		}
		fmt.Printf("  %d) %s%s\n", i+1, name, marker)
	}
	input := promptLine(opts.In, "Base branch [%s]: ", defBase)
	if input == "" {
		return defBase, nil
	}
	if n, err := strconv.Atoi(input); err == nil {
		if n < 1 || n > len(candidates) {
			return "", fmt.Errorf("%s", i18n.T("invalid selection"))
		}
		return candidates[n-1], nil
	}
	if allDigits(input) {
		return "", fmt.Errorf("%s", i18n.T("invalid selection"))
	}
	if !containsName(candidates, input) {
		return "", fmt.Errorf("%s", i18n.T("base branch %q not found", input))
	}
	return input, nil
}

// promptChanges спрашивает, что переносить в новую ветку.
func promptChanges(in io.Reader) (CopyChanges, error) {
	fmt.Println()
	i18n.Printf("What to copy:\n")
	fmt.Printf("  1) %s\n", i18n.T("Uncommitted changes only"))
	fmt.Printf("  2) %s\n", i18n.T("All unique changes"))
	input := promptLine(in, "Select [1]: ")
	switch input {
	case "", "1":
		return CopyUncommitted, nil
	case "2":
		return CopyAll, nil
	default:
		return 0, fmt.Errorf("%s", i18n.T("invalid selection"))
	}
}

// promptLine печатает подсказку и читает строку ввода.
func promptLine(in io.Reader, msgid string, args ...interface{}) string {
	i18n.Printf(msgid, args...)
	line, _ := common.ReadLine(in)
	return strings.TrimSpace(line)
}

// allDigits сообщает, состоит ли строка только из цифр.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// confirm задаёт вопрос с подтверждением по умолчанию «да».
// Пустой ответ, y и yes означают согласие, любой другой — отказ.
func confirm(in io.Reader, msgid string, args ...interface{}) bool {
	switch strings.ToLower(promptLine(in, msgid, args...)) {
	case "", "y", "yes":
		return true
	default:
		return false
	}
}

// switchExisting переключает на существующую локальную ветку.
func switchExisting(cur, target string) error {
	stash, stashed, err := pushStash(target, cur)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("could not stash changes"), err)
	}
	if err := gitStream("switch", target); err != nil {
		if stashed {
			restoreEntry(stash)
		}
		return fmt.Errorf("%s: %w", i18n.T("could not switch to %s", target), err)
	}
	i18n.Green("Switched to branch %s.", target)
	if stashed {
		restoreEntry(stash)
	}
	return nil
}

// switchTracking создаёт локальную ветку из удалённой и переключается на неё.
func switchTracking(cur, target, remote string) error {
	stash, stashed, err := pushStash(target, cur)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("could not stash changes"), err)
	}
	if err := gitStream("switch", "-c", target, "--track", remote); err != nil {
		if stashed {
			restoreEntry(stash)
		}
		return fmt.Errorf("%s: %w", i18n.T("could not switch to %s", target), err)
	}
	i18n.Green("Checked out %s tracking %s.", target, remote)
	if stashed {
		restoreEntry(stash)
	}
	return nil
}

// createBranch создаёт ветку от обновлённой базы и переносит в неё стэш.
func createBranch(cur, target, base string) error {
	baseRef, warn := refreshBase(base)
	if baseRef == "" {
		return fmt.Errorf("%s: %w", i18n.T("base branch %q not found", base), warn)
	}
	if warn != nil {
		i18n.Yellow("Could not fetch %s from origin, using local branch: %v", base, warn)
	}
	stash, stashed, err := pushStash(target, cur)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("could not stash changes"), err)
	}
	// --no-track обязателен: иначе git повесит upstream новой ветки на базовую,
	// и обычный git push может отправить коммиты в неё.
	if err := gitStream("switch", "--no-track", "-c", target, baseRef); err != nil {
		if stashed {
			restoreEntry(stash)
		}
		return fmt.Errorf("%s: %w", i18n.T("could not create branch %s", target), err)
	}
	i18n.Green("Created branch %s from %s.", target, base)
	if stashed {
		restoreEntry(stash)
	}
	return nil
}

// copyToBranch создаёт ветку от базы и переносит в неё выбранные изменения.
func copyToBranch(cur, target, base string, changes CopyChanges) error {
	baseRef, warn := refreshBase(base)
	if baseRef == "" {
		return fmt.Errorf("%s: %w", i18n.T("base branch %q not found", base), warn)
	}
	if warn != nil {
		i18n.Yellow("Could not fetch %s from origin, using local branch: %v", base, warn)
	}

	var commits []string
	if changes == CopyAll {
		var err error
		commits, err = commitsBetween(baseRef, "HEAD")
		if err != nil {
			return err
		}
	}

	stash, stashed, err := pushStash(target, cur)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("could not stash changes"), err)
	}
	if err := gitStream("switch", "--no-track", "-c", target, baseRef); err != nil {
		if stashed {
			restoreEntry(stash)
		}
		return fmt.Errorf("%s: %w", i18n.T("could not create branch %s", target), err)
	}
	i18n.Green("Created branch %s from %s.", target, base)

	for _, sha := range commits {
		conflict, err := cherryPick(sha)
		if err != nil {
			if conflict || cherryPickInProgress() {
				i18n.Red("Cherry-pick stopped at %s.", shortSHA(sha))
				i18n.Yellow("Resolve conflicts, then run: git cherry-pick --continue")
				i18n.Yellow("To abort: git cherry-pick --abort")
				if stashed {
					i18n.Yellow("Uncommitted changes are safe in %s.", stash.Ref)
				}
			}
			return err
		}
	}
	if len(commits) > 0 {
		i18n.Green("Copied commits: %d.", len(commits))
	}
	if stashed {
		restoreEntry(stash)
	}
	return nil
}

// refreshBase возвращает ссылку для ветвления. Если ветка есть на origin,
// она обновляется через fetch. При недоступном origin используется локальная
// ветка, чтобы не ветвиться от устаревшей origin-ссылки; если локальной нет,
// возвращается пустая ссылка и операция прерывается. Второй результат — ошибка
// обновления, при которой работа может продолжаться от локальной ветки.
func refreshBase(base string) (string, error) {
	if remote, ok := remoteBranchRef(base); ok {
		if err := gitStream("fetch", "origin", base); err != nil {
			if localBranchExists(base) {
				return base, err
			}
			return "", err
		}
		return remote, nil
	}
	if localBranchExists(base) {
		return base, nil
	}
	return "", fmt.Errorf("%s", i18n.T("base branch %q not found", base))
}

// commitsBetween возвращает коммиты head, отсутствующие в base, от старых к новым.
func commitsBetween(base, head string) ([]string, error) {
	out, err := gitCapture("rev-list", "--reverse", base+".."+head)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// cherryPick переносит один коммит. Для merge-коммитов берётся основная
// линия. Коммит, ставший пустым после переноса, пропускается.
func cherryPick(sha string) (bool, error) {
	args := []string{"cherry-pick"}
	if isMergeCommit(sha) {
		args = append(args, "-m", "1")
	}
	args = append(args, sha)

	err := gitStream(args...)
	if err == nil {
		return false, nil
	}
	if hasConflicts() {
		return true, err
	}
	// Пустой коммит: git оставил операцию незавершённой, пропускаем её.
	if skipErr := gitStream("cherry-pick", "--skip"); skipErr != nil {
		return false, fmt.Errorf("%w: %s", err,
			i18n.T("cherry-pick state is unclear; run git cherry-pick --abort or --continue"))
	}
	return false, nil
}

// cherryPickInProgress сообщает, осталась ли незавершённая операция cherry-pick.
func cherryPickInProgress() bool {
	_, err := gitCapture("rev-parse", "--verify", "--quiet", "CHERRY_PICK_HEAD")
	return err == nil
}

// isMergeCommit сообщает, имеет ли коммит более одного родителя.
func isMergeCommit(sha string) bool {
	out, err := gitCapture("rev-list", "--parents", "-n", "1", sha)
	if err != nil {
		return false
	}
	return len(strings.Fields(out)) > 2
}

// hasConflicts сообщает, остались ли неразрешённые конфликты индекса.
func hasConflicts() bool {
	out, err := gitCapture("ls-files", "--unmerged")
	return err == nil && out != ""
}

// shortSHA сокращает хэш коммита для вывода.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
