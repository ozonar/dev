package toolchain

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ResolveRuntime возвращает путь к инструменту для запуска или сборки проекта
func ResolveRuntime(language, version string) (string, error) {
	switch language {
	case "php":
		return ensureAndPath(PhpProgram(version))
	case "go":
		return ensureAndPath(GoProgram(version))
	case "node", "javascript":
		if path, ok := systemPresence("npm"); ok {
			return path, nil
		}
		return "", fmt.Errorf("npm not found in PATH (node runtimes are not downloaded automatically)")
	case "python":
		if path, ok := systemPresence("python"); ok {
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

// runVersionCmd выполняет команду определения версии (через sh -c, так как
// команды используют конвейеры и кавычки) и возвращает её вывод без обрамляющих
// пробелов. Пустая строка означает, что версию определить не удалось.
func runVersionCmd(versionCmd string) string {
	cmd := exec.Command("sh", "-c", versionCmd)
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// systemPresence возвращает системный инструмент, если он есть в PATH,
// без проверки версии.
func systemPresence(bin string) (string, bool) {
	path := lookupSystem(bin)
	return path, path != ""
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
func ensureAndPath(ex Executable) (string, error) {
	m, err := NewManager()
	if err != nil {
		return "", err
	}
	programs, err := m.Ensure(ex)
	if err != nil {
		return "", err
	}
	if len(programs) == 0 {
		return "", fmt.Errorf("no program resolved for %s", ex.Name())
	}
	return m.BinaryPath(programs[0]), nil
}
