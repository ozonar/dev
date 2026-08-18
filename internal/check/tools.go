package check

import (
	"fmt"
	"runtime"

	"dev/internal/toolchain"
)

// Program — алиас на описание программы из пакета toolchain.
type Program = toolchain.Program

// programsFor возвращает список линтеров/анализаторов, которые реально
// запускаются в рамках dev check для данного языка. Каждый линтер несёт
// свои зависимости (Require). Версия php прокидывается в зависимости.
func programsFor(language, version string) ([]Program, error) {
	switch language {
	case "go":
		return []Program{goLinter()}, nil
	case "php":
		php := toolchain.PhpProgram(version)
		return []Program{phpStanLinter(php), phpCsFixerLinter(php)}, nil
	default:
		return nil, nil
	}
}

// goLinter описывает golangci-lint.
// Скачивается как tar.gz с GitHub releases (бинарник под ОС/архитектуру).
func goLinter() Program {
	const version = "1.61.0"
	return Program{
		Name:        "golangci-lint",
		Version:     version,
		Binary:      fmt.Sprintf("golangci-lint-%s-%s-%s/golangci-lint", version, runtime.GOOS, runtime.GOARCH),
		URL:         fmt.Sprintf("https://github.com/golangci/golangci-lint/releases/download/v%s/golangci-lint-%s-%s-%s.tar.gz", version, version, runtime.GOOS, runtime.GOARCH),
		Archive:     "tar.gz",
		FullCommand: "{golangci-lint}",
	}
}

// phpStanLinter описывает PHPStan. Требуемый php-рантайм передаётся готовым
// (php), чтобы избежать сетевого запроса при построении линтера.
// Скачивается как phar-файл с GitHub releases, запускается через скачанный php.
func phpStanLinter(php Program) Program {
	return Program{
		Name:        "phpstan",
		Version:     "1.12.0",
		Binary:      "phpstan.phar",
		URL:         "https://github.com/phpstan/phpstan/releases/download/1.12.0/phpstan.phar",
		FullCommand: "{php} {phpstan}",
		Require:     []Program{php},
	}
}

// phpCsFixerLinter описывает PHP CS Fixer. Требуемый php-рантайм передаётся
// готовым (php), чтобы избежать сетевого запроса при построении линтера.
// Скачивается как phar-файл с GitHub releases, запускается через скачанный php.
func phpCsFixerLinter(php Program) Program {
	return Program{
		Name:        "php-cs-fixer",
		Version:     "3.64.0",
		Binary:      "php-cs-fixer.phar",
		URL:         "https://github.com/PHP-CS-Fixer/PHP-CS-Fixer/releases/download/v3.64.0/php-cs-fixer.phar",
		FullCommand: "{php} {php-cs-fixer}",
		Require:     []Program{php},
	}
}

// ensurePrograms гарантирует, что линтеры и их вендоры скачаны.
// Возвращает менеджер инструментов и линтеры для запуска.
func ensurePrograms(language, phpVersion string) (*toolchain.Manager, []Program, error) {
	manager, err := toolchain.NewManager()
	if err != nil {
		return nil, nil, err
	}

	linters, err := programsFor(language, phpVersion)
	if err != nil {
		return nil, nil, err
	}
	if len(linters) == 0 {
		return nil, nil, fmt.Errorf("no linters configured for language %q", language)
	}

	programs, err := manager.Ensure(linters...)
	if err != nil {
		return nil, nil, err
	}

	return manager, programs, nil
}
