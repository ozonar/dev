// Пакет flow реализует git-сценарии переключения между ветками и переноса
// незакоммиченной либо уникальной работы в новую ветку.
package flow

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"dev/internal/i18n"
)

// gitError — ошибка вызова git с сохранённым выводом команды.
type gitError struct {
	args   []string
	output string
	err    error
}

// Error возвращает команду и её вывод, чтобы причина сбоя была видна.
func (e *gitError) Error() string {
	if e.output == "" {
		return fmt.Sprintf("git %s: %v", strings.Join(e.args, " "), e.err)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.args, " "), e.output)
}

// Unwrap возвращает исходную ошибку запуска процесса.
func (e *gitError) Unwrap() error { return e.err }

// gitCapture выполняет git и возвращает stdout без завершающих переводов.
// Вывод ошибки сохраняется в gitError, чтобы пользователь видел причину.
func gitCapture(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", &gitError{args: args, output: strings.TrimSpace(string(out)), err: err}
	}
	return strings.TrimSpace(string(out)), nil
}

// gitStream выполняет git, направляя его вывод прямо в терминал.
// Используется для операций, ход которых важно видеть: switch, stash,
// cherry-pick, fetch.
func gitStream(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return &gitError{args: args, err: err}
	}
	return nil
}

// isGitRepo сообщает, находится ли текущая директория внутри рабочего дерева git.
func isGitRepo() bool {
	out, err := gitCapture("rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// currentBranch возвращает имя текущей ветки. На detached HEAD имя
// отсутствует, поэтому сценарии требуют явного переключения на ветку.
func currentBranch() (string, error) {
	out, err := gitCapture("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || out == "" {
		return "", fmt.Errorf("%s", i18n.T("detached HEAD: switch to a branch first"))
	}
	return out, nil
}

// localBranchExists проверяет наличие локальной ветки.
func localBranchExists(name string) bool {
	_, err := gitCapture("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// remoteBranchRef возвращает ссылку origin/<name>, если ветка есть на origin.
func remoteBranchRef(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	ref := "refs/remotes/origin/" + name
	if _, err := gitCapture("show-ref", "--verify", "--quiet", ref); err == nil {
		return "origin/" + name, true
	}
	return "", false
}

// validateBranchName проверяет имя ветки правилами git.
func validateBranchName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%s", i18n.T("branch name is empty"))
	}
	if _, err := gitCapture("check-ref-format", "--branch", name); err != nil {
		return fmt.Errorf("%s", i18n.T("invalid branch name %q", name))
	}
	return nil
}

// splitLines разбивает вывод git на непустые строки.
func splitLines(s string) []string {
	var result []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}
