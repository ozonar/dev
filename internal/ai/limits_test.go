package ai

import (
	"strconv"
	"strings"
	"testing"
)

// TestPrepareHistory_TrimsLongMessage проверяет, что сообщение длиннее
// MaxMessageLines строк обрезается до последних строк с пометкой о пропуске.
func TestPrepareHistory_TrimsLongMessage(t *testing.T) {
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, "line-"+strconv.Itoa(i))
	}
	history := []HistoryEntry{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: strings.Join(lines, "\n")},
	}

	prepared := prepareHistoryForSend(history)
	if len(prepared) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(prepared))
	}

	content := prepared[1].Content
	if !strings.Contains(content, "(15 lines omitted)") {
		t.Errorf("content should note omitted lines, got: %q", content)
	}
	// Первая строка исходного вывода должна быть отброшена.
	if strings.Contains(content, "line-0") {
		t.Errorf("old lines should be dropped, got: %q", content)
	}
	// Последняя строка исходного вывода должна сохраниться.
	if !strings.Contains(content, "line-29") {
		t.Errorf("last line should be kept, got: %q", content)
	}
	// System-сообщение не должно меняться.
	if prepared[0].Content != "system prompt" {
		t.Errorf("system message changed: %q", prepared[0].Content)
	}
}

// TestPrepareHistory_SmallHistoryUnchanged проверяет, что короткие сообщения
// отправляются без изменений (обрезание по строкам ничего не режет).
func TestPrepareHistory_SmallHistoryUnchanged(t *testing.T) {
	history := []HistoryEntry{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: `[{"command":"ls"}]`},
	}

	prepared := prepareHistoryForSend(history)
	if len(prepared) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(prepared))
	}
	for i, entry := range prepared {
		if entry.Content != history[i].Content || entry.Role != history[i].Role {
			t.Errorf("entry %d changed: %+v vs %+v", i, entry, history[i])
		}
	}
}

// TestPrepareHistory_KeepsSystemAndLast проверяет, что при превышении общего
// лимита символов сохраняются system и последнее сообщение, а суммарный
// размер результата не выходит за MaxRequestChars.
func TestPrepareHistory_KeepsSystemAndLast(t *testing.T) {
	system := "system prompt"
	history := []HistoryEntry{{Role: "system", Content: system}}

	// 6 сообщений по ~30КБ символов — суммарно больше MaxRequestChars.
	longLine := strings.Repeat("x", 2000)
	for i := 0; i < 6; i++ {
		msg := make([]string, MaxMessageLines)
		for j := range msg {
			msg[j] = "msg-" + strconv.Itoa(i) + "-" + longLine
		}
		history = append(history, HistoryEntry{Role: "user", Content: strings.Join(msg, "\n")})
	}

	prepared := prepareHistoryForSend(history)

	if len(prepared) < 2 {
		t.Fatalf("expected at least system and last message, got %d entries", len(prepared))
	}
	if prepared[0].Role != "system" || prepared[0].Content != system {
		t.Errorf("system message should be kept first, got: %+v", prepared[0])
	}
	// Последнее сообщение (актуальный запрос) должно сохраниться.
	last := prepared[len(prepared)-1]
	if !strings.Contains(last.Content, "msg-5-") {
		t.Errorf("last message should be kept, got: %q", last.Content)
	}

	// Суммарный размер не должен превышать лимит.
	total := 0
	for _, entry := range prepared {
		total += len(entry.Content)
	}
	if total > MaxRequestChars {
		t.Errorf("prepared history size %d exceeds limit %d", total, MaxRequestChars)
	}
}

// TestPrepareHistory_Empty проверяет, что пустая история остаётся пустой.
func TestPrepareHistory_Empty(t *testing.T) {
	if got := prepareHistoryForSend(nil); got != nil {
		t.Errorf("expected nil for empty history, got %v", got)
	}
}

// TestClampToChars_Utf8 проверяет, что обрезка по символам не разрывает
// многобайтовые символы UTF-8.
func TestClampToChars_Utf8(t *testing.T) {
	s := "Привет мир"

	if got := ClampToChars(s, 6); got != "Привет" {
		t.Errorf("ClampToChars(6) = %q, want %q", got, "Привет")
	}
	if got := ClampToChars(s, 0); got != "" {
		t.Errorf("ClampToChars(0) = %q, want empty", got)
	}
	if got := ClampToChars(s, 100); got != s {
		t.Errorf("ClampToChars(100) = %q, want %q", got, s)
	}
}
