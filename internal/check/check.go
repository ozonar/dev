package check

import (
	"strings"

	"dev/internal/detector"
	"dev/internal/i18n"
	"dev/internal/toolchain"

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
// Выбор проверок строится по расширениям файлов выбранного объёма,
// а не по типу проекта: для каждого языка, представленного в scope,
// запускаются его линтеры и языко-специфичные проверки (php -l, npm run).
func Run(info *detector.ProjectInfo, opts Options) error {
	// Определяем объём проверки.
	scope, err := resolveScope(opts)
	if err != nil {
		return err
	}

	if len(scope.Files) > 0 {
		i18n.Yellow("Scope: %s (%d files)", scope.Name, len(scope.Files))
	} else {
		i18n.Yellow("Scope: %s (all files)", scope.Name)
	}

	modeLabel := "dry-run"
	if opts.Mode == ModeFix {
		modeLabel = "fix"
	}
	color.Cyan("Mode: %s\n", modeLabel)

	// Языки определяем по расширениям файлов выбранного объёма.
	langs := languagesInFiles(scope.Files)

	// Fallback для полной проверки вне git-репозитория: файлов из git нет,
	// но язык проекта известен — проверяем весь код этого языка.
	if len(langs) == 0 && scope.kind == scopeAll && info.Language != "" && info.Language != "unknown" {
		langs = []string{info.Language}
	}

	if len(langs) == 0 {
		i18n.Yellow("No files with supported extensions found in scope. Nothing to check.")
		return nil
	}

	i18n.Green("Detected languages from files: %s", strings.Join(langs, ", "))

	for _, lang := range langs {
		// Версия из детектора применяется только к языку проекта, чтобы
		// в мультиязычном проекте не подставлять версию чужого языка.
		version := ""
		if info.Language == lang {
			version = info.LanguageVersion
		}
		if err := runLanguage(lang, version, scope, opts.Mode); err != nil {
			i18n.Red("Checks for %s failed: %v", lang, err)
		}
	}

	return nil
}

// runLanguage запускает все проверки одного языка: гарантирует наличие
// линтеров (и их вендоров), прогоняет их по файлам scope и выполняет
// языко-специфичные проверки (php -l, npm run typecheck).
func runLanguage(language, version string, scope Scope, mode Mode) error {
	manager, programs, err := ensurePrograms(language, version)
	if err != nil {
		return err
	}

	i18n.Green("Language: %s", language)

	for _, prog := range programs {
		if _, isRuntime := prog.(toolchain.Runtime); isRuntime {
			continue
		}
		args, ok := buildArgs(prog, scope, mode)
		if !ok {
			i18n.Yellow("No files for %s in scope. Skipping.", prog.Name())
			continue
		}
		printProgramHeader(prog)
		if err := runProgram(manager, prog, args); err != nil {
			i18n.Red("%s finished with error: %v", prog.Name(), err)
		}
	}

	if language == "php" {
		runPhpLint(manager, programs, scope)
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
