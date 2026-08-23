package toolchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addFakeBinary создаёт во временной директории исполняемый файл name со
// скриптом output (выводит его при запуске) и помещает директорию в начало
// PATH. Позволяет замокать системные бинари для проверки резолюции рантайма
// без внешних зависимостей.
func addFakeBinary(t *testing.T, name, output string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"" + output + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })
}

// TestRuntimeUnknown проверяет, что для неизвестного языка нет рантайма.
func TestRuntimeUnknown(t *testing.T) {
	if _, ok := runtimeFor("unknown"); ok {
		t.Errorf("runtimeFor(unknown) = true, want false")
	}
}

// TestSystemProgramPhpVersionMismatch проверяет, что системный php ниже требуемой
// версии не подходит по стратегии satisfiesAtLeast (младшая минорная версия).
func TestSystemProgramPhpVersionMismatch(t *testing.T) {
	// Системный php выдаёт 8.3, а проект требует 8.5.
	addFakeBinary(t, "php", "8.3")
	if NewPhp("").Satisfies("8.3", "8.5") {
		t.Error("Php.Satisfies(8.3, 8.5) = true, want false")
	}
}

// TestSystemProgramPhpVersionOk проверяет, что системный php, удовлетворяющий
// требованию, подходит по стратегии satisfiesAtLeast.
func TestSystemProgramPhpVersionOk(t *testing.T) {
	// Системный php выдаёт 8.6 — подходит под требование 8.5.
	addFakeBinary(t, "php", "8.6")
	if !NewPhp("").Satisfies("8.6", "8.5") {
		t.Error("Php.Satisfies(8.6, 8.5) = false, want true")
	}
}

// TestSystemProgramPhpNoRequirement проверяет, что без требования версии
// системный php подходит безусловно.
func TestSystemProgramPhpNoRequirement(t *testing.T) {
	addFakeBinary(t, "php", "8.3")
	if !NewPhp("").Satisfies("8.3", "") {
		t.Error("Php.Satisfies(8.3, \"\") = false, want true")
	}
}

// TestSystemProgramNodePresence проверяет, что для node достаточно наличия
// инструмента в PATH, без проверки версии.
func TestSystemProgramNodePresence(t *testing.T) {
	// Проверяем node, у которого нет реальной версии в выводе — но он есть.
	addFakeBinary(t, "npm", "not a version")
	path, ok := systemPresence("npm")
	if !ok {
		t.Fatalf("systemPresence(npm) не нашла npm при наличии в PATH")
	}
	if path == "" {
		t.Error("systemPresence(npm) вернула пустой путь при ok=true")
	}
}

// TestSystemProgramPythonAbsent проверяет, что при отсутствии python в PATH
// presence-проверка возвращает false.
func TestSystemProgramPythonAbsent(t *testing.T) {
	// Гарантируем, что python недоступен в PATH, подменяя PATH пустой папкой.
	dir := t.TempDir()
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir)
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })

	path, ok := systemPresence("python")
	if ok {
		t.Errorf("systemPresence(python) = (%q, true), want false при отсутствии python", path)
	}
}

// TestSystemPresence проверяет systemPresence: путь и признак наличия.
func TestSystemPresence(t *testing.T) {
	addFakeBinary(t, "php", "8.6")
	path, ok := systemPresence("php")
	if !ok {
		t.Fatal("systemPresence(php) вернула false при наличии в PATH")
	}
	if path == "" {
		t.Error("systemPresence вернула пустой путь при ok=true")
	}
	if !strings.Contains(path, "php") {
		t.Errorf("path = %q, ожидается путь, содержащий 'php'", path)
	}
}

// TestSystemProgramPhpParsesVersionCmd ловит ошибку в команде определения
// версии php. Команда выполняется через sh -c, и если одинарные кавычки
// в PHP-коде закрываются раньше времени, php получает несколько аргументов и
// завершается с ошибкой (как настоящий php). Fake-бинарь ведёт себя так же:
// требует ровно один аргумент кода после флага -r и возвращает версию только
// в этом случае.
func TestSystemProgramPhpParsesVersionCmd(t *testing.T) {
	// Кастомный fake php: валиден только когда после -r передан один аргумент
	// кода (корректная команда). Иначе — parse error, как у настоящего php.
	script := `#!/bin/sh
if [ "$1" = "-r" ] && [ "$#" -eq 2 ]; then
	echo "8.3"
	exit 0
fi
echo "PHP Parse error: syntax error" >&2
exit 255
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "php"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })

	m := newTestManager(t)
	cand, ok := NewPhp("8.3").Resolve(m.dir, "8.3")
	if !ok {
		t.Fatalf("не найден подходящий php: команда определения версии некорректна")
	}
	rt, isRT := cand.(Runtime)
	if !isRT || !rt.IsSystem() || rt.Version() != "8.3" {
		t.Fatalf("ожидался системный php 8.3, получен %v", m.BinaryPath(cand))
	}
}
