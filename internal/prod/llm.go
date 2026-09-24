package prod

import (
	"encoding/json"

	"dev/internal/ai"
)

// LLMOptions — параметры запроса к LLM.
type LLMOptions struct {
	Endpoint string
	Token    string
	Model    string
}

// GenerateLLMReport отправляет отчёт в LLM через общий клиент ai.Client
// и возвращает развёрнутый анализ. Формат запроса/ответа и авторетрай
// на временные ошибки реализованы в ai.Client.
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

	history := []ai.HistoryEntry{
		{Role: "system", Content: "You are a senior SRE engineer. Answer in Russian."},
		{Role: "user", Content: prompt},
	}

	client := &ai.Client{Endpoint: opts.Endpoint, Token: opts.Token, Model: opts.Model}
	return client.Chat(history, 0.2)
}
