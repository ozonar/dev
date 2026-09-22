package prod

import (
	"encoding/json"
	"testing"
)

// TestChatResponseLLM_ErrorAsString проверяет, что ответ с полем error в виде
// строки (типично для прокси) парсится без падения.
func TestChatResponseLLM_ErrorAsString(t *testing.T) {
	body := `{"error":"Proxy error: Failure when receiving data from the peer"}`
	var resp chatResponseLLM
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal must not fail on string error: %v", err)
	}
	if len(resp.Error) == 0 {
		t.Errorf("expected non-empty raw error field")
	}
}

// TestChatResponseLLM_ErrorAsObject проверяет парсинг классического объекта
// ошибки вида {"error":{"message":"..."}}.
func TestChatResponseLLM_ErrorAsObject(t *testing.T) {
	body := `{"error":{"message":"Invalid API key"}}`
	var resp chatResponseLLM
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal must not fail on object error: %v", err)
	}
	if len(resp.Error) == 0 {
		t.Errorf("expected non-empty raw error field")
	}
}

// TestChatResponseLLM_NormalBody проверяет, что обычный успешный ответ парсится
// так же, как раньше, и поле error остаётся пустым.
func TestChatResponseLLM_NormalBody(t *testing.T) {
	body := `{"choices":[{"message":{"content":"ok"}}]}`
	var resp chatResponseLLM
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal of normal body failed: %v", err)
	}
	if len(resp.Error) != 0 {
		t.Errorf("expected no error, got %s", string(resp.Error))
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "ok" {
		t.Errorf("expected one choice with content, got %+v", resp.Choices)
	}
}
