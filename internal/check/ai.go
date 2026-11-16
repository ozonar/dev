package check

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dev/internal/ai"
	"dev/internal/detector"
	"dev/internal/i18n"
)

// Ошибки, означающие, что AI-ревью не пройдено.
var (
	// ErrCriticalFindings — модель обнаружила критические проблемы.
	ErrCriticalFindings = errors.New("AI review found critical issues")
	// ErrVerdictUnknown — модель не сообщила вердикт, поэтому гарантировать
	// отсутствие критических проблем нельзя.
	ErrVerdictUnknown = errors.New("AI review did not report a verdict")
)

// RunAI выполняет AI-код-ревью изменённого кода.
// info — уже определённая информация о проекте (детекция и принудительный
// язык из флагов выполняются на уровне команд перед вызовом).
// Возвращает ErrCriticalFindings или ErrVerdictUnknown, если ревью не пройдено.
func RunAI(info *detector.ProjectInfo, opts Options, instruction string) error {
	i18n.Green("Project: %s (%s)", info.Language, info.Framework)

	// Определяем объём проверки.
	scope := resolveScopeForAI(opts)

	// Формируем текст для отправки на ревью в зависимости от объёма.
	text := scope.GetChanges()

	// Если отправить нечего — выходим.
	if text == "" {
		i18n.Yellow("Nothing to send. Aborting review.")
		return nil
	}

	// Ограничиваем размер отправляемого кода общим лимитом символов на ревью.
	// Обрезаем по рунам, чтобы не разорвать многобайтовый символ UTF-8.
	if len([]rune(text)) > ai.MaxReviewTotalChars {
		i18n.Yellow("Code exceeds %d characters, truncating.", ai.MaxReviewTotalChars)
		text = ai.ClampToChars(text, ai.MaxReviewTotalChars)
	}

	verdict, err := ai.RunCodeReview(text, instruction)
	if err != nil {
		return fmt.Errorf(i18n.T("AI review failed: %v"), err)
	}

	switch verdict {
	case ai.VerdictOK:
		return nil
	case ai.VerdictCritical:
		return ErrCriticalFindings
	default:
		return ErrVerdictUnknown
	}
}

// resolveScopeForAI определяет объём для AI-ревью: явный Scope из флагов,
// интерактивный выбор без варианта "весь код", либо default.
func resolveScopeForAI(opts Options) Scope {
	if opts.Scope != nil {
		return *opts.Scope
	}
	if opts.Interactive {
		return promptScopeForAI()
	}
	// Без интерактивности берём default (изменённый код или всё, если нет git).
	return ScopeDefault()
}

// readScopeFiles читает содержимое файлов из списка и объединяет его в текст.
// Пропускает бинарные файлы и файлы, размер которых превышает общий лимит
// MaxReviewFileSize: такие файлы (например, изображения) в ревью не попадают.
// Размер проверяется через os.Stat до чтения, чтобы не загружать в память
// потенциально огромные файлы.
func readScopeFiles(files []string) string {
	var sb strings.Builder

	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			// Файл мог быть удалён или это директория — пропускаем.
			continue
		}
		if info.Size() > ai.MaxReviewFileSize {
			// Слишком большой файл исключаем ещё до чтения.
			continue
		}

		data, err := os.ReadFile(f)
		if err != nil {
			// Файл мог быть удалён между stat и чтением — пропускаем.
			continue
		}
		if strings.ContainsRune(string(data), 0) {
			// Признак бинарного файла — пропускаем.
			continue
		}

		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "===== %s =====\n", filepath.ToSlash(f))
		sb.Write(data)
	}

	return sb.String()
}
