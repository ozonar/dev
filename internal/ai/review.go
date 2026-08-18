package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/fatih/color"
)

// buildReviewPrompt формирует системный промпт для код-ревью.
// Просит LLM не выдумывать проблем, которых нет, и фокусироваться
// на реально критических местах.
func buildReviewPrompt(text string) string {
	return fmt.Sprintf(`Ты — опытный senior-ревьюер кода. Ты ТОЛЬКО анализируешь предоставленный код. Ты ничего не исправлял и не менял.
Проанализируй код и честно перечисли РЕАЛЬНЫЕ проблемы, которые видишь в нём.

САМОЕ ВАЖНОЕ ПРАВИЛО — БУДЬ ЧЕСТНЫМ:
- Никогда не утверждай, что что-то «исправлено», «улучшено», «добавлено» или «выполнено», ЕСЛИ ты этого не видишь в коде. Ты не автор изменений и ничего не правил.
- Если видишь ошибку, опечатку или баг — честно укажи её
- Категорически НЕ выдумывай проблем, которых нет. Не приписывай коду несуществующие свойства.
- Если код корректен и проблем нет — так и напиши, не сочиняя их.

Формат ответа (простым текстом, без JSON):
1. **Критические проблемы** — баги, утечки ресурсов, проблемы безопасности, ошибки бизнес-логики, сломанная компиляция. Если нет — напиши 'не обнаружено'.
2. **Потенциальные проблемы** — то, что может сломаться при определённых условиях или в будущем.
3. **Стиль и поддерживаемость** — только если это реально мешает читаемости/поддержке.

Правила:
- Опирайся ТОЛЬКО на предоставленный код. Не придумывай контекст.
- Для каждой проблемы укажи файл (если известен) и краткое описание.
- Отделяй действительно критичное от косметики.

=== Код для ревью ===
%s`, text)
}

// queryReviewText отправляет текстовый запрос к LLM и возвращает ответ текстом.
// В отличие от queryLLM (который ждёт JSON-массив команд), эта функция
// принимает произвольный текстовый ответ.
func queryReviewText(cfg *Config, history []HistoryEntry) (string, error) {
	messages := make([]chatMessage, len(history))
	for i, entry := range history {
		messages[i] = chatMessage{
			Role:    entry.Role,
			Content: entry.Content,
		}
	}

	reqBody := chatRequest{
		Model:       cfg.Model,
		Messages:    messages,
		Temperature: 0.2,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	curlCmd := exec.Command("curl", "-s",
		"-k",
		"-X", "POST",
		cfg.Endpoint,
		"-H", "Content-Type: application/json",
		"-H", "Authorization: Bearer "+cfg.Token,
		"-d", string(jsonData),
	)

	var stdout, stderr bytes.Buffer
	curlCmd.Stdout = &stdout
	curlCmd.Stderr = &stderr

	if err := curlCmd.Run(); err != nil {
		return "", fmt.Errorf("curl failed: %w\nStderr: %s", err, stderr.String())
	}

	var resp chatResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w\nBody: %s", err, stdout.String())
	}

	if resp.Error != nil {
		return "", fmt.Errorf("API error: %s", resp.Error.Message)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// renderMarkdown применяет базовое markdown-форматирование к строке:
// **жирный** текст и `инлайн-код`.
func renderMarkdown(md string) string {
	bold := color.New(color.Bold).SprintFunc()
	code := color.New(color.FgCyan).SprintFunc()

	// Сначала инлайн-код, затем жирный.
	withCode := replaceInline(md, "`", func(s string) string { return code(s) })
	return replaceInline(withCode, "**", func(s string) string { return bold(s) })
}

// replaceInline оборачивает текст между парными маркерами marker в wrap.
// Если пара не закрыта — оставляет текст как есть.
func replaceInline(s, marker string, wrap func(string) string) string {
	var sb strings.Builder
	rest := s
	for {
		open := strings.Index(rest, marker)
		if open < 0 {
			sb.WriteString(rest)
			break
		}
		close := strings.Index(rest[open+len(marker):], marker)
		if close < 0 {
			sb.WriteString(rest)
			break
		}
		close += open + len(marker)

		sb.WriteString(rest[:open])
		sb.WriteString(wrap(rest[open+len(marker) : close]))
		rest = rest[close+len(marker):]
	}
	return sb.String()
}

// RunCodeReview выполняет AI-код-ревью переданного кода.
// text — строка с изменениями (diff) либо полным содержимым изменённых файлов.
func RunCodeReview(text, instruction string) (string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		color.Red("Config error: %v", err)
		if err := InteractiveEditConfig(); err != nil {
			return "", err
		}
		cfg, err = LoadConfig()
		if err != nil {
			return "", fmt.Errorf("config still invalid after edit: %w", err)
		}
	}

	history := []HistoryEntry{
		{Role: "system", Content: buildReviewPrompt(text)},
		{Role: "user", Content: "Проведи код-ревью предоставленного кода."},
		{Role: "user", Content: strings.TrimSpace(instruction)},
	}

	color.Cyan("Sending code to LLM for review...")
	review, err := queryReviewText(cfg, history)
	if err != nil {
		return "", err
	}

	color.Green("\n=== AI Code Review ===")
	fmt.Println(renderMarkdown(review))
	fmt.Println()

	return review, nil
}
