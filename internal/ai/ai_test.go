package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestChatResponse_ErrorAsString проверяет, что ответ API с полем error в виде
// строки (так отвечают прокси, например OpenRouter) парсится без ошибки и не
// роняет последующую обработку ответа.
func TestChatResponse_ErrorAsString(t *testing.T) {
	body := `{"error":"Proxy error: Failure when receiving data from the peer"}`
	var resp chatResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal must not fail on string error: %v", err)
	}
	if len(resp.Error) == 0 {
		t.Errorf("expected non-empty raw error field")
	}
	if len(resp.Choices) != 0 {
		t.Errorf("expected no choices, got %d", len(resp.Choices))
	}
}

// TestChatResponse_ErrorAsObject проверяет парсинг классического объекта ошибки
// вида {"error":{"message":"..."}}.
func TestChatResponse_ErrorAsObject(t *testing.T) {
	body := `{"error":{"message":"Invalid API key"}}`
	var resp chatResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal must not fail on object error: %v", err)
	}
	if len(resp.Error) == 0 {
		t.Errorf("expected non-empty raw error field")
	}
}

// TestChatResponse_NormalBody проверяет, что обычный успешный ответ парсится
// так же, как раньше, и поле error остаётся пустым.
func TestChatResponse_NormalBody(t *testing.T) {
	body := `{"choices":[{"message":{"content":"[{\"command\":\"ls\"}]"}}]}`
	var resp chatResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal of normal body failed: %v", err)
	}
	if len(resp.Error) != 0 {
		t.Errorf("expected no error, got %s", string(resp.Error))
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content == "" {
		t.Errorf("expected one choice with content")
	}
}

// TestRunShellCommand_Success проверяет, что shell-команда с "!" выполняется
// и её результат добавляется в историю диалога.
func TestRunShellCommand_Success(t *testing.T) {
	var history []HistoryEntry

	// Выполняем простую команду через "!"-префикс
	input := "!echo hello-from-dev"
	if err := runShellCommand(&history, input); err != nil {
		t.Fatalf("runShellCommand returned error: %v", err)
	}

	// В историю должна добавиться ровно одна запись от assistant
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry := history[0]
	if entry.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", entry.Role)
	}

	// В содержимое записи должны попасть сама команда и её вывод
	if !strings.Contains(entry.Content, "echo hello-from-dev") {
		t.Errorf("history entry should contain the executed command, got: %q", entry.Content)
	}
	if !strings.Contains(entry.Content, "hello-from-dev") {
		t.Errorf("history entry should contain command output, got: %q", entry.Content)
	}
}

// TestRunShellCommand_EmptyCommand проверяет, что пустая команда после "!"
// не добавляет запись в историю и не возвращает ошибку.
func TestRunShellCommand_EmptyCommand(t *testing.T) {
	var history []HistoryEntry

	// Только символ "!" без команды
	if err := runShellCommand(&history, "!"); err != nil {
		t.Fatalf("runShellCommand returned error for empty command: %v", err)
	}
	if err := runShellCommand(&history, "!   "); err != nil {
		t.Fatalf("runShellCommand returned error for whitespace command: %v", err)
	}

	if len(history) != 0 {
		t.Fatalf("expected no history entries for empty commands, got %d", len(history))
	}
}

// TestRunShellCommand_Error проверяет, что команда, завершившаяся с ошибкой,
// добавляет в историю запись с информацией об ошибке.
func TestRunShellCommand_Error(t *testing.T) {
	var history []HistoryEntry

	// Команда, которая гарантированно завершится с ошибкой
	input := "!false"
	if err := runShellCommand(&history, input); err != nil {
		t.Fatalf("runShellCommand returned error: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry := history[0]
	if entry.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", entry.Role)
	}

	// Запись должна содержать пометку об ошибке
	if !strings.Contains(entry.Content, "Error") {
		t.Errorf("history entry should mention error, got: %q", entry.Content)
	}
}

// TestAppendSendAnalysis_NoAnalysis проверяет, что пока analysis-команды
// не выполнялись, пункт SEND_ANALYSIS в список не добавляется.
func TestAppendSendAnalysis_NoAnalysis(t *testing.T) {
	commands := []CommandAction{{Command: "ls", Type: CommandTypeSequence}}
	got := appendSendAnalysis(commands, false)

	if len(got) != 1 {
		t.Fatalf("expected unchanged list length 1, got %d", len(got))
	}
	if got[0].Command == "SEND_ANALYSIS" {
		t.Errorf("SEND_ANALYSIS must not be added before analysis executes")
	}
}

// TestAppendSendAnalysis_EmptyList проверяет главный сценарий бага: после
// выполнения единственной analysis-команды (список опустел) пункт
// SEND_ANALYSIS всё равно должен появиться, чтобы пользователь мог
// отправить результаты анализа в LLM.
func TestAppendSendAnalysis_EmptyList(t *testing.T) {
	got := appendSendAnalysis(nil, true)

	if len(got) != 1 {
		t.Fatalf("expected exactly one SEND_ANALYSIS command, got %d", len(got))
	}
	if got[0].Command != "SEND_ANALYSIS" {
		t.Errorf("expected SEND_ANALYSIS command, got %q", got[0].Command)
	}
	if got[0].Description == "" {
		t.Errorf("SEND_ANALYSIS should have a human-readable description")
	}
}

// TestAppendSendAnalysis_AppendsToEnd проверяет, что SEND_ANALYSIS добавляется
// в конец списка оставшихся команд.
func TestAppendSendAnalysis_AppendsToEnd(t *testing.T) {
	commands := []CommandAction{
		{Command: "ls", Type: CommandTypeSequence},
		{Command: "sudo nginx -t", Type: CommandTypeSequence},
	}
	got := appendSendAnalysis(commands, true)

	if len(got) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(got))
	}
	if got[2].Command != "SEND_ANALYSIS" {
		t.Errorf("expected SEND_ANALYSIS at the end, got %q at index 2", got[2].Command)
	}
}

// TestAppendSendAnalysis_NoDuplicate проверяет, что если SEND_ANALYSIS уже
// есть в списке (например, LLM сам его вернул), дубликат не добавляется.
func TestAppendSendAnalysis_NoDuplicate(t *testing.T) {
	commands := []CommandAction{
		{Command: "ls", Type: CommandTypeSequence},
		{Command: "SEND_ANALYSIS"},
	}
	got := appendSendAnalysis(commands, true)

	if len(got) != 2 {
		t.Fatalf("expected no duplicate, got %d commands", len(got))
	}
}
