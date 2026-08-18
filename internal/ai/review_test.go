package ai

import (
	"strings"
	"testing"
)

// TestBuildReviewPrompt_ContainsText проверяет, что промпт содержит
// переданный текст для ревью.
func TestBuildReviewPrompt_ContainsText(t *testing.T) {
	text := "package foo\n\nfunc Foo() {}"
	prompt := buildReviewPrompt(text)

	// Промпт должен содержать инструкцию не выдумывать проблемы.
	if !strings.Contains(prompt, "НЕ выдумывай") {
		t.Errorf("prompt should instruct the LLM not to invent problems")
	}
	// Промпт должен содержать переданный код.
	if !strings.Contains(prompt, "package foo") {
		t.Errorf("prompt should contain the review text")
	}
}

// TestBuildReviewPrompt_EmptyText проверяет, что промпт корректен при
// пустом тексте (промпт собирается, но код отсутствует).
func TestBuildReviewPrompt_EmptyText(t *testing.T) {
	prompt := buildReviewPrompt("")

	if !strings.Contains(prompt, "Код для ревью") {
		t.Errorf("prompt should contain the code section header")
	}
	if strings.Count(prompt, "=== Код для ревью ===") != 1 {
		t.Errorf("prompt should contain exactly one code section")
	}
}

// TestRenderMarkdown_Bold проверяет, что **жирный** текст заменяется на ANSI.
func TestRenderMarkdown_Bold(t *testing.T) {
	got := renderMarkdown("1. **Критические проблемы** — не обнаружено.")

	if !strings.Contains(got, "\x1b[") {
		t.Errorf("renderMarkdown should contain ANSI escape for bold, got: %q", got)
	}
	// Сырые маркеры ** должны исчезнуть.
	if strings.Contains(got, "**") {
		t.Errorf("renderMarkdown should strip ** markers, got: %q", got)
	}
	// Сам текст остаётся.
	if !strings.Contains(got, "Критические проблемы") {
		t.Errorf("renderMarkdown should keep the text, got: %q", got)
	}
}

// TestRenderMarkdown_NoBold проверяет, что без ** текст не меняется.
func TestRenderMarkdown_NoBold(t *testing.T) {
	got := renderMarkdown("просто текст без разметки")

	if got != "просто текст без разметки" {
		t.Errorf("renderMarkdown should keep plain text unchanged, got: %q", got)
	}
}

// TestRenderMarkdown_Unpaired проверяет, что незакрытая ** не ломает текст.
func TestRenderMarkdown_Unpaired(t *testing.T) {
	got := renderMarkdown("текст с **незакрытой парой")

	// Незакрытая пара должна остаться как есть (без ANSI).
	if strings.Contains(got, "\x1b[") {
		t.Errorf("renderMarkdown should not render unpaired **, got: %q", got)
	}
	if !strings.Contains(got, "**") {
		t.Errorf("renderMarkdown should keep unpaired ** markers, got: %q", got)
	}
}

// TestRenderMarkdown_InlineCode проверяет, что `инлайн-код` красится.
func TestRenderMarkdown_InlineCode(t *testing.T) {
	got := renderMarkdown("Вызов `foo()` и `bar()` внутри.")

	if !strings.Contains(got, "\x1b[") {
		t.Errorf("renderMarkdown should add ANSI for inline code, got: %q", got)
	}
	// Маркеры бэктиков должны исчезнуть.
	if strings.Contains(got, "`") {
		t.Errorf("renderMarkdown should strip backticks, got: %q", got)
	}
	if !strings.Contains(got, "foo()") || !strings.Contains(got, "bar()") {
		t.Errorf("renderMarkdown should keep inline code text, got: %q", got)
	}
}
