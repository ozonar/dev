package check

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dev/internal/ai"
	"dev/internal/detector"
	"dev/internal/i18n"
)

// RunAI выполняет AI-код-ревью изменённого кода.
// info — уже определённая информация о проекте (детекция и принудительный
// язык из флагов выполняются на уровне команд перед вызовом).
func RunAI(info *detector.ProjectInfo, opts Options, instruction string) error {
	i18n.Green("Project: %s (%s)", info.Language, info.Framework)

	// Определяем объём проверки.
	scope, err := resolveScopeForAI(opts)
	if err != nil {
		return err
	}

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

	if _, err := ai.RunCodeReview(text, instruction); err != nil {
		return fmt.Errorf(i18n.T("AI review failed: %v"), err)
	}

	return nil
}

// resolveScopeForAI определяет объём для AI-ревью: явный Scope из флагов,
// интерактивный выбор без варианта "весь код", либо default.
func resolveScopeForAI(opts Options) (Scope, error) {
	if opts.Scope != nil {
		return *opts.Scope, nil
	}
	if opts.Interactive {
		return promptScopeForAI(), nil
	}
	// Без интерактивности берём default (изменённый код или всё, если нет git).
	return ScopeDefault(), nil
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
			// Файл слишком большой — исключаем из ревью целиком, не читая его.
			i18n.Yellow("Skipping file %s: size %d exceeds limit of %d bytes",
				f, info.Size(), ai.MaxReviewFileSize)
			continue
		}

		data, err := os.ReadFile(f)
		if err != nil {
			// Файл мог быть удалён между stat и чтением — пропускаем.
			continue
		}
		if strings.ContainsRune(string(data), 0) {
			// Признак бинарного файла — пропускаем.
			i18n.Yellow("Skipping binary file: %s", f)
			continue
		}

		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("===== %s =====\n", filepath.ToSlash(f)))
		sb.Write(data)
	}

	return sb.String()
}
