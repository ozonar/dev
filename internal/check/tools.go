package check

import (
	"fmt"
	"runtime"

	"dev/internal/toolchain"
)

// programsFor возвращает список линтеров/анализаторов, которые реально
// запускаются в рамках dev check для данного языка. Каждый линтер несёт
// свои зависимости (Require). Версия php прокидывается в зависимости.
// Для javascript (Node/TypeScript) используется Biome, для python — Ruff.
func programsFor(language, version string) ([]toolchain.Executable, error) {
	switch language {
	case "go":
		goRT := toolchain.GoProgram(version)
		return []toolchain.Executable{goLinter(goRT)}, nil
	case "php":
		php := toolchain.PhpProgram(version)
		return []toolchain.Executable{phpStanLinter(php), phpCsFixerLinter(php)}, nil
	case "javascript":
		return []toolchain.Executable{biomeLinter()}, nil
	case "python":
		return []toolchain.Executable{ruffLinter()}, nil
	default:
		return nil, nil
	}
}

// goLinter описывает golangci-lint.
// Скачивается как tar.gz с GitHub releases (бинарник под ОС/архитектуру).
func goLinter(goRT toolchain.Executable) toolchain.Executable {
	const version = "1.61.0"
	binary := fmt.Sprintf("golangci-lint-%s-%s-%s/golangci-lint", version, runtime.GOOS, runtime.GOARCH)
	url := fmt.Sprintf("https://github.com/golangci/golangci-lint/releases/download/v%s/golangci-lint-%s-%s-%s.tar.gz", version, version, runtime.GOOS, runtime.GOARCH)
	return toolchain.NewProgram("golangci-lint", version, binary, url, "tar.gz", "{golangci-lint}", goRT)
}

// phpStanLinter описывает PHPStan. Требуемый php-рантайм передаётся готовым
// (php), чтобы избежать сетевого запроса при построении линтера.
// Скачивается как phar-файл с GitHub releases, запускается через скачанный php.
func phpStanLinter(php toolchain.Executable) toolchain.Executable {
	url := "https://github.com/phpstan/phpstan/releases/download/1.12.0/phpstan.phar"
	return toolchain.NewProgram("phpstan", "1.12.0", "phpstan.phar", url, "", "{php} {phpstan}", php)
}

// phpCsFixerLinter описывает PHP CS Fixer. Требуемый php-рантайм передаётся
// готовым (php), чтобы избежать сетевого запроса при построении линтера.
// Скачивается как phar-файл с GitHub releases, запускается через скачанный php.
func phpCsFixerLinter(php toolchain.Executable) toolchain.Executable {
	url := "https://github.com/PHP-CS-Fixer/PHP-CS-Fixer/releases/download/v3.64.0/php-cs-fixer.phar"
	return toolchain.NewProgram("php-cs-fixer", "3.64.0", "php-cs-fixer.phar", url, "", "{php} {php-cs-fixer}", php)
}

// biomeLinter описывает Biome — анализатор для JavaScript/TypeScript,
// объединяющий линтер, форматтер и импорт-сортировку. Скачивается как
// одиночный исполняемый файл с GitHub releases (бинарник под ОС/архитектуру).
func biomeLinter() toolchain.Executable {
	const version = "2.5.9"
	suffix := archSuffix(runtime.GOARCH)
	binary := "biome-linux-" + suffix
	url := fmt.Sprintf("https://github.com/biomejs/biome/releases/download/@biomejs/biome@%s/biome-linux-%s", version, suffix)
	return toolchain.NewProgram("biome", version, binary, url, "", "{biome}")
}

// ruffLinter описывает Ruff — анализатор для Python, объединяющий линтер,
// форматтер и иморт-сортировку. Скачивается как tar.gz с GitHub releases
// (бинарник под ОС/архитектуру), внутри архива — папка с бинарём.
func ruffLinter() toolchain.Executable {
	const version = "0.16.3"
	arch := ruffArch(runtime.GOARCH)
	binary := "ruff-" + arch + "/ruff"
	url := fmt.Sprintf("https://github.com/astral-sh/ruff/releases/download/%s/ruff-%s.tar.gz", version, arch)
	return toolchain.NewProgram("ruff", version, binary, url, "tar.gz", "{ruff}")
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
func ensurePrograms(language, phpVersion string) (*toolchain.Manager, []toolchain.Executable, error) {
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
