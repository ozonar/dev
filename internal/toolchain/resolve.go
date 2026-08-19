package toolchain

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ResolveRuntime возвращает путь к инструменту для запуска или сборки проекта
// на заданном языке. Сначала проверяется системный инструмент: если он доступен
// и его версия удовлетворяет требованию проекта — используется он. Иначе для php
// и go инструмент скачивается в dev-command. Для node и python скачивание не
// выполняется — возвращается понятная ошибка.
func ResolveRuntime(language, version string) (string, error) {
	if path, ok := resolveSystem(language, version); ok {
		// Используем системный (локально установленный) рантайм.
		fmt.Printf("Using local %s\n", language)
		return path, nil
	}

	switch language {
	case "php":
		path, err := ensureAndPath(PhpProgram(version))
		if err != nil {
			return "", err
		}
		fmt.Printf("Using downloaded %s %s\n", language, version)
		return path, nil
	case "go":
		path, err := ensureAndPath(GoProgram(version))
		if err != nil {
			return "", err
		}
		fmt.Printf("Using downloaded %s %s\n", language, version)
		return path, nil
	case "node", "javascript":
		return "", fmt.Errorf("npm not found in PATH (node runtimes are not downloaded automatically)")
	case "python":
		return "", fmt.Errorf("python not found in PATH (python runtimes are not downloaded automatically)")
	default:
		return "", fmt.Errorf("unsupported runtime language %q", language)
	}
}

// ResolveLnav возвращает путь к lnav. Если системный lnav доступен —
// используется он. Иначе lnav скачивается в dev-command и возвращается его путь.
func ResolveLnav() (string, error) {
	if path := lookupSystem("lnav"); path != "" {
		return path, nil
	}
	return ensureAndPath(LnavProgram())
}

// resolveSystem находит системный инструмент для языка. Для php и go требуется,
// чтобы версия удовлетворяла требованию проекта; для node и python достаточно
// наличия инструмента в PATH.
func resolveSystem(language, required string) (string, bool) {
	switch language {
	case "php":
		return systemSatisfies("php", "php -r 'echo PHP_MAJOR_VERSION.'.'.PHP_MINOR_VERSION;'", required)
	case "go":
		return systemSatisfies("go", "go version | awk '{print $3}' | sed 's/^go//'", required)
	case "node", "javascript":
		return systemPresence("npm")
	case "python":
		return systemPresence("python")
	default:
		return "", false
	}
}

// systemPresence возвращает системный инструмент, если он есть в PATH,
// без проверки версии.
func systemPresence(bin string) (string, bool) {
	path := lookupSystem(bin)
	return path, path != ""
}

// systemSatisfies проверяет наличие бинаря в PATH и, при заданной required
// версии, соответствие версии системного инструмента требованию проекта.
// Если required пустая — системный инструмент подходит безусловно.
func systemSatisfies(bin, versionCmd, required string) (string, bool) {
	path := lookupSystem(bin)
	if path == "" {
		return "", false
	}
	if required == "" {
		return path, true
	}

	// Команды определения версии используют конвейеры и кавычки, поэтому
	// выполняются через sh -c.
	cmd := exec.Command("sh", "-c", versionCmd)
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	system := strings.TrimSpace(string(out))
	if system == "" || compareMajorMinor(system, required) < 0 {
		return "", false
	}
	return path, true
}

// lookupSystem ищет исполняемый файл в PATH. Пустая строка означает,
// что инструмент в системе отсутствует.
func lookupSystem(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

// ensureAndPath гарантирует наличие программы в dev-command и возвращает путь
// к её исполняемому файлу.
func ensureAndPath(p Program) (string, error) {
	m, err := NewManager()
	if err != nil {
		return "", err
	}
	programs, err := m.Ensure(p)
	if err != nil {
		return "", err
	}
	if len(programs) == 0 {
		return "", fmt.Errorf("no program resolved for %s", p.Name)
	}
	return m.BinaryPath(programs[0]), nil
}
