package flow

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"dev/internal/i18n"
)

type branch struct {
	Name       string
	LastCommit time.Time
	AuthorMail string
	Mine       bool
}

// listLocalBranches читает локальные ветки одним вызовом git.
func listLocalBranches() ([]branch, error) {
	out, err := gitCapture("for-each-ref",
		"--format=%(refname:short)%09%(committerdate:unix)%09%(authoremail)",
		"refs/heads")
	if err != nil {
		return nil, err
	}
	me := userEmail()
	var result []branch
	for _, line := range splitLines(out) {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			return nil, fmt.Errorf("unexpected git for-each-ref output: %q", line)
		}
		unix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected commit date %q for branch %s", parts[1], parts[0])
		}
		mail := normalizeEmail(parts[2])
		result = append(result, branch{
			Name:       parts[0],
			LastCommit: time.Unix(unix, 0),
			AuthorMail: mail,
			Mine:       me != "" && strings.EqualFold(mail, me),
		})
	}
	return result, nil
}

// listRemoteNames возвращает имена веток origin без префикса origin/.
func listRemoteNames() ([]string, error) {
	out, err := gitCapture("for-each-ref", "--format=%(refname:short)", "refs/remotes/origin")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, ref := range splitLines(out) {
		name, ok := strings.CutPrefix(ref, "origin/")
		if !ok || name == "HEAD" {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// userEmail возвращает email текущего пользователя git.
// Пустая строка означает, что email не настроен: «своих» веток тогда нет.
func userEmail() string {
	out, err := gitCapture("config", "user.email")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// normalizeEmail убирает угловые скобки, которые git добавляет к email.
func normalizeEmail(s string) string {
	return strings.Trim(strings.TrimSpace(s), "<>")
}

// defaultBranch определяет ветку по умолчанию: origin/HEAD, затем main или
// master, затем текущая ветка.
func defaultBranch() (string, error) {
	if out, err := gitCapture("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if name, ok := strings.CutPrefix(out, "origin/"); ok && name != "" {
			return name, nil
		}
	}
	for _, name := range []string{"main", "master"} {
		if localBranchExists(name) {
			return name, nil
		}
		if _, ok := remoteBranchRef(name); ok {
			return name, nil
		}
	}
	return currentBranch()
}

// isRootBranch сообщает, относится ли ветка к корневым: ветка по умолчанию,
// develop или release-ветка.
func isRootBranch(name, def string) bool {
	return name == def || name == "develop" || strings.HasPrefix(name, "release")
}

// recentBranches выбирает недавние некорневые ветки и сортирует их:
// сначала ветки текущего пользователя, внутри групп — по дате коммита.
func recentBranches(branches []branch, def string, since time.Time, limit int) []branch {
	var out []branch
	for _, b := range branches {
		if isRootBranch(b.Name, def) || b.LastCommit.Before(since) {
			continue
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Mine != out[j].Mine {
			return out[i].Mine
		}
		return out[i].LastCommit.After(out[j].LastCommit)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// allBranchNames объединяет локальные и удалённые ветки без дублей.
func allBranchNames() ([]string, error) {
	locals, err := listLocalBranches()
	if err != nil {
		return nil, err
	}
	remotes, err := listRemoteNames()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(locals)+len(remotes))
	var names []string
	for _, b := range locals {
		if !seen[b.Name] {
			seen[b.Name] = true
			names = append(names, b.Name)
		}
	}
	for _, name := range remotes {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}

// baseCandidates возвращает кандидатов для точки ветвления в порядке
// приоритета: develop, ветка по умолчанию, release-ветки.
func baseCandidates(names []string, def string) []string {
	var hasDevelop, hasDef bool
	var releases []string
	for _, name := range names {
		switch {
		case name == "develop":
			hasDevelop = true
		case name == def:
			hasDef = true
		case strings.HasPrefix(name, "release"):
			releases = append(releases, name)
		}
	}
	sort.Strings(releases)
	var out []string
	if hasDevelop {
		out = append(out, "develop")
	}
	if hasDef {
		out = append(out, def)
	}
	return append(out, releases...)
}

// defaultBase выбирает базовую ветку по умолчанию: develop (если он есть и
// предпочтителен), иначе ветка по умолчанию.
func defaultBase(def string, candidates []string, preferDevelop bool) string {
	if preferDevelop {
		for _, name := range candidates {
			if name == "develop" {
				return name
			}
		}
	}
	for _, name := range candidates {
		if name == def {
			return name
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return def
}

// containsName сообщает, есть ли имя в списке.
func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// humanSince форматирует время последнего коммита в короткий относительный вид.
func humanSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return i18n.T("just now")
	case d < time.Hour:
		return i18n.T("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return i18n.T("%dh ago", int(d.Hours()))
	default:
		return i18n.T("%dd ago", int(d.Hours()/24))
	}
}
