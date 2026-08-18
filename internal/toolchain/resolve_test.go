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

// TestResolveSystemUnknown проверяет, что для неизвестного языка резолюция
// возвращает пустой результат.
func TestResolveSystemUnknown(t *testing.T) {
	path, ok := resolveSystem("unknown", "1.0")
	if ok {
		t.Errorf("resolveSystem(unknown) = (%q, true), want false", path)
	}
}

// TestResolveSystemPhpVersionMismatch проверяет, что при системном PHP ниже
// требуемой версии инструмент не используется (резолюция вернёт false),
// что приводит к скачиванию требуемой версии.
func TestResolveSystemPhpVersionMismatch(t *testing.T) {
	// Системный php выдаёт 8.3, а проект требует 8.5.
	addFakeBinary(t, "php", "8.3")
	path, ok := resolveSystem("php", "8.5")
	if ok {
		t.Errorf("resolveSystem(php, 8.5) = (%q, true), want false (системная 8.3 < 8.5)", path)
	}
}

// TestResolveSystemPhpVersionOk проверяет, что системный PHP, удовлетворяющий
// требованию, используется (резолюция возвращает путь).
func TestResolveSystemPhpVersionOk(t *testing.T) {
	// Системный php выдаёт 8.6 — подходит под требование 8.5.
	addFakeBinary(t, "php", "8.6")
	path, ok := resolveSystem("php", "8.5")
	if !ok {
		t.Fatalf("resolveSystem(php, 8.5) не нашла подходящий системный php")
	}
	if path == "" {
		t.Error("resolveSystem вернула пустой путь при ok=true")
	}
}

// TestResolveSystemPhpNoRequirement проверяет, что без требования версии
// системный php подходит безусловно.
func TestResolveSystemPhpNoRequirement(t *testing.T) {
	addFakeBinary(t, "php", "8.3")
	path, ok := resolveSystem("php", "")
	if !ok {
		t.Fatalf("resolveSystem(php, \"\") не нашла системный php")
	}
	if path == "" {
		t.Error("resolveSystem вернула пустой путь при ok=true")
	}
}

// TestResolveSystemNodePresence проверяет, что для node достаточно наличия
// инструмента в PATH, без проверки версии.
func TestResolveSystemNodePresence(t *testing.T) {
	// Проверяем node, у которого нет реальной версии в выводе — но он есть.
	addFakeBinary(t, "npm", "not a version")
	path, ok := resolveSystem("node", "18")
	if !ok {
		t.Fatalf("resolveSystem(node) не нашла npm при наличии в PATH")
	}
	if path == "" {
		t.Error("resolveSystem(node) вернула пустой путь при ok=true")
	}
}

// TestResolveSystemPythonAbsent проверяет, что при отсутствии python резолюция
// возвращает false.
func TestResolveSystemPythonAbsent(t *testing.T) {
	// Гарантируем, что python недоступен в PATH, подменяя PATH пустой папкой.
	dir := t.TempDir()
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir)
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })

	path, ok := resolveSystem("python", "")
	if ok {
		t.Errorf("resolveSystem(python) = (%q, true), want false при отсутствии python", path)
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
