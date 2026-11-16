package ai

import (
	"fmt"
	"regexp"
	"strings"

	"dev/internal/i18n"

	"github.com/fatih/color"
)

// Verdict — итог AI-ревью по критическим проблемам.
type Verdict int

const (
	// VerdictUnknown — модель не сообщила вердикт в ожидаемом формате.
	VerdictUnknown Verdict = iota
	// VerdictOK — критических проблем не обнаружено.
	VerdictOK
	// VerdictCritical — обнаружены критические проблемы.
	VerdictCritical
)

// ReviewResult — результат AI-ревью.
type ReviewResult struct {
	Text    string  // полный текст ответа модели
	Verdict Verdict // итог по критическим проблемам
}

// verdictRe ищет строку вердикта в ответе модели. Маркер запрашивается
// промптом на английском и не зависит от языка текста ревью.
var verdictRe = regexp.MustCompile(`(?mi)^\s*VERDICT\s*:\s*(CRITICAL|OK)\s*$`)

// parseVerdict извлекает вердикт из ответа модели. Если маркер встречается
// несколько раз, решающей считается последняя строка: промпт требует
// вердикт в самом конце ответа.
func parseVerdict(review string) Verdict {
	matches := verdictRe.FindAllStringSubmatch(review, -1)
	if len(matches) == 0 {
		return VerdictUnknown
	}
	switch strings.ToUpper(matches[len(matches)-1][1]) {
	case "CRITICAL":
		return VerdictCritical
	case "OK":
		return VerdictOK
	default:
		return VerdictUnknown
	}
}

// printVerdict выводит итог ревью по критическим проблемам.
func printVerdict(v Verdict) {
	switch v {
	case VerdictCritical:
		i18n.Red("AI review verdict: CRITICAL")
	case VerdictOK:
		i18n.Green("AI review verdict: OK")
	default:
		i18n.Yellow("AI review verdict: unknown (the model did not report a verdict)")
	}
}

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

Обязательное завершение ответа — отдельная последняя строка с вердиктом:
VERDICT: CRITICAL — если обнаружена хотя бы одна критическая проблема;
VERDICT: OK — если критических проблем нет.
После строки вердикта ничего не пиши.

=== Код для ревью ===
%s`, text)
}

// queryReviewText отправляет текстовый запрос к LLM через общий клиент
// и возвращает ответ текстом. В отличие от queryLLM (который ждёт
// JSON-массив команд), здесь принимается произвольный текстовый ответ.
// Авторетрай на временные ошибки выполняет клиент.
func queryReviewText(cfg *Config, history []HistoryEntry) (string, error) {
	history = prepareHistoryForSend(history)
	return NewClient(cfg).Chat(history, 0.2)
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
func RunCodeReview(text, instruction string) (*ReviewResult, error) {
	cfg, err := LoadConfig()
	if err != nil {
		i18n.Red("Config error: %v", err)
		if err := InteractiveEditConfig(); err != nil {
			return nil, err
		}
		cfg, err = LoadConfig()
		if err != nil {
			return nil, fmt.Errorf(i18n.T("config still invalid after edit: %w"), err)
		}
	}

	history := []HistoryEntry{
		{Role: "system", Content: buildReviewPrompt(text)},
		{Role: "user", Content: "Проведи код-ревью предоставленного кода."},
		{Role: "user", Content: strings.TrimSpace(instruction)},
	}

	i18n.Cyan("Sending code to LLM for review...")
	review, err := queryReviewText(cfg, history)
	if err != nil {
		return nil, err
	}

	fmt.Println()
	i18n.Green("=== AI Code Review ===")
	fmt.Println(renderMarkdown(review))
	fmt.Println()

	result := &ReviewResult{Text: review, Verdict: parseVerdict(review)}
	printVerdict(result.Verdict)
	return result, nil
}
