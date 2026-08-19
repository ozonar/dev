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
// Для javascript (Node/TypeScript) используется Biome, для python — Ruff.
func programsFor(language, version string) ([]Program, error) {
	switch language {
	case "go":
		return []Program{goLinter()}, nil
	case "php":
		php := toolchain.PhpProgram(version)
		return []Program{phpStanLinter(php), phpCsFixerLinter(php)}, nil
	case "javascript":
		return []Program{biomeLinter()}, nil
	case "python":
		return []Program{ruffLinter()}, nil
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

// biomeLinter описывает Biome — анализатор для JavaScript/TypeScript,
// объединяющий линтер, форматтер и импорт-сортировку. Скачивается как
// одиночный исполняемый файл с GitHub releases (бинарник под ОС/архитектуру).
func biomeLinter() Program {
	const version = "2.5.9"
	return Program{
		Name:        "biome",
		Version:     version,
		Binary:      "biome-linux-" + archSuffix(runtime.GOARCH),
		URL:         fmt.Sprintf("https://github.com/biomejs/biome/releases/download/@biomejs/biome@%s/biome-linux-%s", version, archSuffix(runtime.GOARCH)),
		Archive:     "",
		FullCommand: "{biome}",
	}
}

// ruffLinter описывает Ruff — анализатор для Python, объединяющий линтер,
// форматтер и иморт-сортировку. Скачивается как tar.gz с GitHub releases
// (бинарник под ОС/архитектуру), внутри архива — папка с бинарём.
func ruffLinter() Program {
	const version = "0.16.3"
	arch := ruffArch(runtime.GOARCH)
	return Program{
		Name:        "ruff",
		Version:     version,
		Binary:      "ruff-" + arch + "/ruff",
		URL:         fmt.Sprintf("https://github.com/astral-sh/ruff/releases/download/%s/ruff-%s.tar.gz", version, arch),
		Archive:     "tar.gz",
		FullCommand: "{ruff}",
	}
}

// archSuffix возвращает суффикс архитектуры для артефактов Biome.
// Поддерживаются x64 и arm64; остальные архитектуры не предусмотрены.
func archSuffix(arch string) string {
	if arch == "arm64" {
		return "arm64"
	}
	return "x64"
}

// ruffArch возвращает целевую платформу для артефактов Ruff
// (x86_64 или aarch64 на linux). Прочие ОС не поддерживаются.
func ruffArch(arch string) string {
	if arch == "arm64" {
		return "aarch64-unknown-linux-gnu"
	}
	return "x86_64-unknown-linux-gnu"
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
