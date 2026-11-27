package flow

import (
	"fmt"
	"strings"

	"dev/internal/i18n"
)

// stashMarker — префикс сообщения стэша. Сообщение является единственным
// источником правды о том, для какой ветки отложены изменения.
const stashMarker = "dev-flow: "

// stashEntry описывает запись в списке стэшей.
type stashEntry struct {
	Ref     string
	Message string
}

// stashMessage формирует сообщение стэша для целевой ветки.
func stashMessage(target, source string) string {
	if source == "" {
		return stashMarker + target
	}
	return fmt.Sprintf("%s%s (from %s)", stashMarker, target, source)
}

// stashLabel возвращает префикс сообщения для целевой ветки.
func stashLabel(target string) string {
	return stashMarker + target
}

// stashMatches сообщает, относится ли сообщение стэша к целевой ветке.
// git формирует subject стэша как "On <ветка>: <сообщение>", поэтому
// проверяется вхождение маркера с границей после имени ветки.
func stashMatches(message, target string) bool {
	label := stashLabel(target)
	idx := strings.Index(message, label)
	if idx < 0 {
		return false
	}
	rest := message[idx+len(label):]
	return rest == "" || strings.HasPrefix(rest, " (from ")
}

// worktreeDirty сообщает, есть ли незакоммиченные изменения (включая untracked).
func worktreeDirty() (bool, error) {
	out, err := gitCapture("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// pushStash сохраняет все незакоммиченные изменения под меткой целевой ветки.
// Если сохранять нечего, возвращает ok=false. Только что созданный стэш
// всегда находится под ссылкой stash@{0}.
func pushStash(target, source string) (stashEntry, bool, error) {
	dirty, err := worktreeDirty()
	if err != nil {
		return stashEntry{}, false, err
	}
	if !dirty {
		return stashEntry{}, false, nil
	}
	msg := stashMessage(target, source)
	if err := gitStream("stash", "push", "--include-untracked", "-m", msg); err != nil {
		return stashEntry{}, false, err
	}
	// Список нужен лишь для точной ссылки: даже при сбое чтения изменения
	// уже сохранены в свежем stash@{0}, поэтому код не теряется.
	entries, err := listStashes()
	if err == nil && len(entries) > 0 && stashMatches(entries[0].Message, target) {
		return entries[0], true, nil
	}
	return stashEntry{Ref: "stash@{0}", Message: msg}, true, nil
}

// listStashes читает стэши в порядке от новых к старым.
func listStashes() ([]stashEntry, error) {
	out, err := gitCapture("stash", "list", "--format=%gd%x09%s")
	if err != nil {
		return nil, err
	}
	var entries []stashEntry
	for _, line := range splitLines(out) {
		ref, msg, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("unexpected git stash list output: %q", line)
		}
		entries = append(entries, stashEntry{Ref: ref, Message: msg})
	}
	return entries, nil
}

// findStash ищет самый свежий стэш, отложенный для указанной ветки.
func findStash(target string) (stashEntry, bool, error) {
	entries, err := listStashes()
	if err != nil {
		return stashEntry{}, false, err
	}
	for _, e := range entries {
		if stashMatches(e.Message, target) {
			return e, true, nil
		}
	}
	return stashEntry{}, false, nil
}

// applyStash применяет стэш и удаляет его только после успешного применения.
// При конфликте стэш остаётся: изменения не теряются.
func applyStash(e stashEntry) error {
	if err := gitStream("stash", "apply", e.Ref); err != nil {
		return fmt.Errorf("%s", i18n.T("could not apply stash %s; it is kept", e.Ref))
	}
	if err := gitStream("stash", "drop", e.Ref); err != nil {
		return fmt.Errorf("%s", i18n.T("stash %s applied but not removed", e.Ref))
	}
	return nil
}

// restoreEntry возвращает отложенные изменения в рабочее дерево.
// Ошибка не прерывает сценарий: код остаётся в стэше, пользователь получает
// инструкцию, как вернуть его вручную.
func restoreEntry(e stashEntry) {
	i18n.Cyan("Restoring stashed changes...")
	if err := applyStash(e); err != nil {
		i18n.Red("%v", err)
		i18n.Yellow("Your changes are safe in %s. Restore manually: git stash apply %s", e.Ref, e.Ref)
		return
	}
	i18n.Green("Stashed changes restored.")
}

// restoreStash возвращает изменения, отложенные для указанной ветки, если они есть.
func restoreStash(target string) {
	e, ok, err := findStash(target)
	if err != nil {
		i18n.Red("Could not read stash list: %v", err)
		return
	}
	if !ok {
		return
	}
	restoreEntry(e)
}
