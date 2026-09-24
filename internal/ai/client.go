package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"dev/internal/i18n"
)

// chatRequest — запрос к OpenAI-совместимому API.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

// chatResponse — ответ API.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error json.RawMessage `json:"error,omitempty"`
}

// Client — HTTP-клиент к OpenAI-совместимому API. Единственный владелец
// формата запроса/ответа; используется диалогом (dev ai), код-ревью
// (dev review) и прод-отчётами (prod llm).
type Client struct {
	Endpoint string
	Token    string
	Model    string
}

// NewClient создаёт клиента из конфигурации.
func NewClient(cfg *Config) *Client {
	return &Client{Endpoint: cfg.Endpoint, Token: cfg.Token, Model: cfg.Model}
}

// Chat отправляет историю сообщений и возвращает текстовый ответ модели.
// Временные сбои (ошибка curl, непарсируемый ответ, API-ошибка, пустой ответ)
// обрабатываются авторетраем — до 3 попыток.
func (c *Client) Chat(history []HistoryEntry, temperature float64) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		messages := make([]chatMessage, len(history))
		for i, entry := range history {
			messages[i] = chatMessage(entry)
		}

		reqBody := chatRequest{
			Model:       c.Model,
			Messages:    messages,
			Temperature: temperature,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return "", fmt.Errorf(i18n.T("failed to marshal request: %w"), err)
		}

		curlCmd := exec.Command("curl", "-s",
			"-k",
			"-X", "POST",
			c.Endpoint,
			"-H", "Content-Type: application/json",
			"-H", "Authorization: Bearer "+c.Token,
			"-d", string(jsonData),
		)

		var stdout, stderr bytes.Buffer
		curlCmd.Stdout = &stdout
		curlCmd.Stderr = &stderr

		if err := curlCmd.Run(); err != nil {
			return "", fmt.Errorf(i18n.T("curl failed: %w\nStderr: %s"), err, stderr.String())
		}

		var resp chatResponse
		if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
			lastErr = fmt.Errorf(i18n.T("unparsable response: %w\nBody: %s"), err, stdout.String())
			i18n.Red("LLM returned unparsable response (attempt %d/3)", attempt+1)
			continue
		}

		// API-ошибка. Поле error бывает строкой или объектом; здесь важен сам
		// факт ошибки, а не формат — повторяем запрос, такие ошибки часто временные.
		if len(resp.Error) > 0 {
			lastErr = fmt.Errorf(i18n.T("API error: %s"), strings.TrimSpace(string(resp.Error)))
			i18n.Red("LLM API error (attempt %d/3)", attempt+1)
			continue
		}

		if len(resp.Choices) == 0 {
			lastErr = fmt.Errorf(i18n.T("empty response from API"))
			i18n.Red("LLM returned empty response (attempt %d/3)", attempt+1)
			continue
		}

		return strings.TrimSpace(resp.Choices[0].Message.Content), nil
	}

	return "", fmt.Errorf(i18n.T("LLM request failed after 3 attempts: %w"), lastErr)
}
