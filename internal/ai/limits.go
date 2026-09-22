package ai

// Общие лимиты на отправку данных в LLM. Используются и в диалоге (dev ai),
// и в код-ревью (dev review), чтобы запросы не превышали контекст модели.
const (
	// MaxMessageLines — максимальное количество строк в одном сообщении диалога.
	// Если сообщение длиннее, к LLM отправляются только последние строки,
	// так как свежий вывод важнее самого начала.
	MaxMessageLines = 15

	// MaxRequestChars — общий лимит символов запроса к LLM (суммарно по всем
	// сообщениям). При превышении самые старые не-system сообщения отбрасываются.
	MaxRequestChars = 150000

	// MaxReviewFileSize — максимальный размер одного файла (в байтах) для
	// включения в ревью. Файлы больше этого размера исключаются из ревью целиком:
	// это защищает от изображений и других больших файлов, которые не являются
	// исходным кодом.
	MaxReviewFileSize = 512 * 1024 // 512 КБ

	// MaxReviewTotalChars — лимит символов текста кода для ревью. Значение
	// меньше MaxRequestChars, чтобы в запрос вместе с кодом поместились ещё
	// системный промпт и служебные сообщения.
	MaxReviewTotalChars = 100000
)

// prepareHistoryForSend подготавливает историю диалога к отправке в LLM:
//  1. Каждое сообщение (кроме system) обрезается до MaxMessageLines последних
//     строк, чтобы одно сообщение не раздувало контекст.
//  2. Если суммарный объём превышает MaxRequestChars символов, отбрасываются
//     самые старые не-system сообщения. Первое (system) и последнее (актуальный
//     запрос) сообщения сохраняются всегда, поэтому порядок диалога не ломается.
//
// Исходный срез не модифицируется — возвращается новая история.
func prepareHistoryForSend(history []HistoryEntry) []HistoryEntry {
	if len(history) == 0 {
		return history
	}

	// Шаг 1: обрезаем каждое сообщение до MaxMessageLines строк.
	trimmed := make([]HistoryEntry, len(history))
	for i, entry := range history {
		if i == 0 {
			trimmed[i] = entry
			continue
		}
		trimmed[i] = HistoryEntry{Role: entry.Role, Content: truncateOutput(entry.Content, MaxMessageLines)}
	}

	// Шаг 2: суммарный размер в пределах лимита — отправляем как есть.
	total := 0
	for _, entry := range trimmed {
		total += len(entry.Content)
	}
	if total <= MaxRequestChars {
		return trimmed
	}

	// Шаг 3: лимит превышен. Гарантированно сохраняем system и последнее
	// сообщение, из остальных оставляем самые свежие, пока они влезают в бюджет.
	first := trimmed[0]
	last := trimmed[len(trimmed)-1]

	// Если system-сообщение само превышает лимит — обрезаем его, сохраняя
	// начало, где находятся инструкции.
	if len(first.Content) > MaxRequestChars {
		first.Content = clampToChars(first.Content, MaxRequestChars)
	}

	budget := MaxRequestChars - len(first.Content)
	if budget <= 0 {
		// System-сообщение занимает весь лимит — остальные сообщения не помещаются.
		return []HistoryEntry{first}
	}

	if len(last.Content) > budget {
		last.Content = clampToChars(last.Content, budget)
	}
	budget -= len(last.Content)

	var middle []HistoryEntry
	for i := len(trimmed) - 2; i >= 1; i-- {
		if len(trimmed[i].Content) > budget {
			continue
		}
		middle = append([]HistoryEntry{trimmed[i]}, middle...)
		budget -= len(trimmed[i].Content)
	}

	result := make([]HistoryEntry, 0, len(middle)+2)
	result = append(result, first)
	result = append(result, middle...)
	result = append(result, last)
	return result
}

// clampToChars обрезает строку до max символов (рун), не разрывая
// многобайтовые символы UTF-8.
func clampToChars(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
