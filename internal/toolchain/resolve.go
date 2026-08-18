package toolchain

import (
	"fmt"
	"os/exec"
)

// ResolveRuntime возвращает путь к инструменту для запуска или сборки проекта
// на заданном языке. Если системный инструмент доступен — используется он.
// Для php и go при отсутствии системного инструмент скачивается в dev-command.
// Для node и python скачивание не выполняется — возвращается понятная ошибка.
func ResolveRuntime(language, version string) (string, error) {
	switch language {
	case "php":
		if path := lookupSystem("php"); path != "" {
			return path, nil
		}
		return ensureAndPath(PhpProgram(version))
	case "go":
		if path := lookupSystem("go"); path != "" {
			return path, nil
		}
		return ensureAndPath(GoProgram(version))
	case "node", "javascript":
		if path := lookupSystem("npm"); path != "" {
			return path, nil
		}
		return "", fmt.Errorf("npm not found in PATH (node runtimes are not downloaded automatically)")
	case "python":
		if path := lookupSystem("python"); path != "" {
			return path, nil
		}
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
