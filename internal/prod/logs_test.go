package prod

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"dev/internal/colors"
)

// TestCoreLogGlobs проверяет стандартные места логов для core-служб.
func TestCoreLogGlobs(t *testing.T) {
	// Известная служба возвращает свои шаблоны.
	if globs := coreLogGlobs("nginx"); len(globs) == 0 {
		t.Error("ожидались шаблоны логов для nginx")
	}
	// Версионированный php-fpm использует те же пути, что и php-fpm.
	phpGlobs := coreLogGlobs("php-fpm")
	if globs := coreLogGlobs("php8.3-fpm"); !reflect.DeepEqual(globs, phpGlobs) {
		t.Errorf("php8.3-fpm должен использовать шаблоны php-fpm, получено %v", globs)
	}
	// Неизвестная служба — nil.
	if globs := coreLogGlobs("unknown-service"); globs != nil {
		t.Errorf("ожидался nil, получено %v", globs)
	}
}

// TestFindLogFiles проверяет поиск *.log файлов с исключениями.
func TestFindLogFiles(t *testing.T) {
	dir := t.TempDir()
	// Обычные лог-файлы, которые должны быть найдены.
	if err := os.WriteFile(filepath.Join(dir, "app.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "error.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Исключаемые директории — их логи не должны попасть в результат.
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "skip.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "ignore.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Не-лог файл.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := findLogFiles(dir)
	if len(got) != 2 {
		t.Fatalf("ожидалось 2 лог-файла, получено %d: %v", len(got), got)
	}
	for _, f := range got {
		if strings.Contains(f, "node_modules") || strings.Contains(f, ".git") {
			t.Errorf("не должно быть логов из исключённых директорий: %s", f)
		}
	}
}

// TestServiceLabel проверяет цветную подпись службы.
func TestServiceLabel(t *testing.T) {
	// Активная служба — зелёная с временем работы.
	active := &ServiceInfo{Name: "nginx", State: ServiceActive, Uptime: 2 * time.Hour}
	label := ServiceLabel(active)
	if !strings.Contains(label, colors.ColorGreen) {
		t.Errorf("активная служба должна быть зелёной, получено %q", label)
	}
	if !strings.Contains(label, "nginx") || !strings.Contains(label, "2h") {
		t.Errorf("подпись должна содержать имя и время работы, получено %q", label)
	}
	// Упавшая служба — красная с меткой failed.
	failed := &ServiceInfo{Name: "redis", State: ServiceFailed}
	label = ServiceLabel(failed)
	if !strings.Contains(label, colors.ColorRed) {
		t.Errorf("упавшая служба должна быть красной, получено %q", label)
	}
	if !strings.Contains(label, "failed") {
		t.Errorf("подпись должна содержать метку failed, получено %q", label)
	}
}
