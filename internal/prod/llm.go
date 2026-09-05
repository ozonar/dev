package prod

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// chatMessageLLM — сообщение для OpenAI-compatible API.
type chatMessageLLM struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequestLLM — запрос к API.
type chatRequestLLM struct {
	Model       string           `json:"model"`
	Messages    []chatMessageLLM `json:"messages"`
	Temperature float64          `json:"temperature"`
}

// chatResponseLLM — ответ API.
type chatResponseLLM struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// LLMOptions — параметры запроса к LLM.
type LLMOptions struct {
	Endpoint string
	Token    string
	Model    string
}

// GenerateLLMReport отправляет отчёт в LLM и возвращает развёрнутый анализ.
func GenerateLLMReport(opts LLMOptions, rep *Report) (string, error) {
	reportJSON, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", err
	}

	prompt := `Ты — SRE-инженер, анализирующий инцидент на продакшене.
Ниже представлен отчёт о состоянии сервера (JSON).
Определи вероятную первопричину проблемы, построй причинную цепочку и
дай рекомендации по устранению. Отвечай структурированно и кратко.

Отчёт:
` + string(reportJSON)

	messages := []chatMessageLLM{
		{Role: "system", Content: "You are a senior SRE engineer. Answer in Russian."},
		{Role: "user", Content: prompt},
	}

	reqBody := chatRequestLLM{
		Model:       opts.Model,
		Messages:    messages,
		Temperature: 0.2,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	curlCmd := exec.Command("curl", "-s", "-k", "-X", "POST", opts.Endpoint,
		"-H", "Content-Type: application/json",
		"-H", "Authorization: Bearer "+opts.Token,
		"-d", string(jsonData),
	)
	var stdout, stderr bytes.Buffer
	curlCmd.Stdout = &stdout
	curlCmd.Stderr = &stderr
	if err := curlCmd.Run(); err != nil {
		return "", fmt.Errorf("curl failed: %w\nStderr: %s", err, stderr.String())
	}

	var resp chatResponseLLM
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return "", fmt.Errorf("parse response: %w\nBody: %s", err, stdout.String())
	}
	if resp.Error != nil {
		return "", fmt.Errorf("API error: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from API")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}
