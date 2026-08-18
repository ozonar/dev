package check

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dev/internal/ai"
	"dev/internal/detector"

	"github.com/fatih/color"
)

// maxCodeSize — максимальный суммарный размер отправляемого кода (символов),
// чтобы не превысить лимит контекста LLM.
const maxCodeSize = 100000

// RunAI выполняет AI-код-ревью изменённого кода.
func RunAI(root string, opts Options, instruction string) error {
	info, err := detector.DetectProject(root)
	if err != nil {
		return fmt.Errorf("failed to detect project: %v", err)
	}

	color.Green("Project: %s (%s)", info.Language, info.Framework)

	// Определяем объём проверки.
	scope, err := resolveScopeForAI(opts)
	if err != nil {
		return err
	}

	// Формируем текст для отправки на ревью в зависимости от объёма.
	text := scope.GetChanges()

	// Если отправить нечего — выходим.
	if text == "" {
		color.Yellow("Nothing to send. Aborting review.")
		return nil
	}

	if _, err := ai.RunCodeReview(text, instruction); err != nil {
		return fmt.Errorf("AI review failed: %v", err)
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
// Пропускает бинарные и слишком большие файлы. Обрезает по maxCodeSize.
func readScopeFiles(files []string) string {
	var sb strings.Builder
	total := 0

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			// Файл мог быть удалён или это директория — пропускаем.
			continue
		}
		if strings.ContainsRune(string(data), 0) {
			// Признак бинарного файла — пропускаем.
			color.Yellow("Skipping binary file: %s", f)
			continue
		}

		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("===== %s =====\n", filepath.ToSlash(f)))
		sb.Write(data)

		total += len(data)
		if total > maxCodeSize {
			color.Yellow("Code exceeds %d characters, truncating.", maxCodeSize)
			break
		}
	}

	return sb.String()
}
