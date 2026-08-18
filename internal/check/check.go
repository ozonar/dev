package check

import (
	"fmt"

	"dev/internal/detector"

	"github.com/fatih/color"
)

// Options — параметры запуска команды dev check.
type Options struct {
	Mode Mode // ModeDryRun (по умолчанию) или ModeFix
	// Scope — явно заданный объём проверки. Если nil, объём определяется
	// автоматически: интерактивным выбором либо дефолтным.
	Scope *Scope
	// Interactive — запросить объём у пользователя, если Scope не задан.
	Interactive bool
}

// Run выполняет статическую проверку кода анализаторами.
func Run(root string, opts Options) error {
	info, err := detector.DetectProject(root)
	if err != nil {
		return fmt.Errorf("failed to detect project: %v", err)
	}

	if info.Language == "" || info.Language == "unknown" {
		return fmt.Errorf("unsupported project language: %q", info.Language)
	}

	color.Green("Project: %s (%s)", info.Language, info.Framework)

	// Скачиваем линтеры и их вендоры (Require), если ещё не скачаны.
	manager, programs, err := ensurePrograms(info.Language, info.LanguageVersion)
	if err != nil {
		return fmt.Errorf("failed to prepare tools: %v", err)
	}

	// Определяем объём проверки.
	scope, err := resolveScope(opts)
	if err != nil {
		return err
	}

	if len(scope.Files) > 0 {
		color.Yellow("Scope: %s (%d files)", scope.Name, len(scope.Files))
	} else {
		color.Yellow("Scope: %s (all files)", scope.Name)
	}

	modeLabel := "dry-run"
	if opts.Mode == ModeFix {
		modeLabel = "fix"
	}
	color.Cyan("Mode: %s\n", modeLabel)

	// Запускаем все программы.
	for _, prog := range programs {
		printProgramHeader(prog)
		args := buildArgs(prog, scope, opts.Mode)
		if err := runProgram(manager, prog, args); err != nil {
			color.Red("%s finished with error: %v", prog.Name, err)
		}
	}

	return nil
}

// resolveScope определяет объём проверки на основе опций.
func resolveScope(opts Options) (Scope, error) {
	if opts.Scope != nil {
		return *opts.Scope, nil
	}
	if opts.Interactive {
		return promptScope(), nil
	}
	return ScopeDefault(), nil
}
