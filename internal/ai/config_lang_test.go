package ai

import (
	"os"
	"path/filepath"
	"testing"
)

// writeHomeConfig создаёт временный HOME с файлом ~/dev-config/main.conf.
func writeHomeConfig(t *testing.T, content string) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "dev-config")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.conf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
}

func TestLanguageFromConfig(t *testing.T) {
	writeHomeConfig(t, "# комментарий\nLANGUAGE=ru\nLLM_ENDPOINT=x\n")
	if got := LanguageFromConfig(); got != "ru" {
		t.Errorf("LanguageFromConfig() = %q, want ru", got)
	}
}

func TestLanguageFromConfigAuto(t *testing.T) {
	// Конфиг есть, но ключа LANGUAGE нет — автоопределение (пустая строка).
	writeHomeConfig(t, "LLM_ENDPOINT=x\n")
	if got := LanguageFromConfig(); got != "" {
		t.Errorf("LanguageFromConfig() = %q, want empty (auto)", got)
	}
}

func TestLanguageFromConfigMissing(t *testing.T) {
	// HOME без конфига — пустая строка (автоопределение),
	// если нет и системного конфига в /etc/dev-command.
	t.Setenv("HOME", t.TempDir())
	if got := LanguageFromConfig(); got != "" {
		t.Errorf("LanguageFromConfig() = %q, want empty", got)
	}
}

func TestLoadConfigParsesLanguage(t *testing.T) {
	writeHomeConfig(t, "LANGUAGE=en\nLLM_ENDPOINT=e\nLLM_TOKEN=t\nLLM_MODEL=m\n")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Language != "en" {
		t.Errorf("cfg.Language = %q, want en", cfg.Language)
	}
}
