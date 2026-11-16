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
	t.Setenv("LANGUAGE", "")
	writeHomeConfig(t, "# комментарий\nLANGUAGE=ru\nLLM_ENDPOINT=x\n")
	if got := LanguageFromConfig(); got != "ru" {
		t.Errorf("LanguageFromConfig() = %q, want ru", got)
	}
}

func TestLanguageFromConfigAuto(t *testing.T) {
	// Конфиг есть, но ключа LANGUAGE нет — автоопределение (пустая строка).
	t.Setenv("LANGUAGE", "")
	writeHomeConfig(t, "LLM_ENDPOINT=x\n")
	if got := LanguageFromConfig(); got != "" {
		t.Errorf("LanguageFromConfig() = %q, want empty (auto)", got)
	}
}

func TestLanguageFromConfigMissing(t *testing.T) {
	// HOME без конфига — пустая строка (автоопределение),
	// если нет и системного конфига в /etc/dev-command.
	t.Setenv("LANGUAGE", "")
	t.Setenv("HOME", t.TempDir())
	if got := LanguageFromConfig(); got != "" {
		t.Errorf("LanguageFromConfig() = %q, want empty", got)
	}
}

// TestLoadConfigFromEnv проверяет, что параметры могут быть заданы только
// окружением: в CI конфиг-файла нет.
func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LLM_ENDPOINT", "env-endpoint")
	t.Setenv("LLM_TOKEN", "env-token")
	t.Setenv("LLM_MODEL", "env-model")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Endpoint != "env-endpoint" || cfg.Token != "env-token" || cfg.Model != "env-model" {
		t.Errorf("LoadConfig() = %+v, want values from environment", cfg)
	}
}

// TestLoadConfigEnvOverridesFile проверяет единое правило: окружение
// приоритетнее файла, остальные значения берутся из файла.
func TestLoadConfigEnvOverridesFile(t *testing.T) {
	writeHomeConfig(t, "LLM_ENDPOINT=file-e\nLLM_TOKEN=file-t\nLLM_MODEL=file-m\n")
	t.Setenv("LLM_TOKEN", "env-token")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Endpoint != "file-e" {
		t.Errorf("Endpoint = %q, want file-e", cfg.Endpoint)
	}
	if cfg.Token != "env-token" {
		t.Errorf("Token = %q, want env-token", cfg.Token)
	}
	if cfg.Model != "file-m" {
		t.Errorf("Model = %q, want file-m", cfg.Model)
	}
}

// TestLoadConfigIncompleteEnv проверяет, что неполный набор переменных
// без файла — ошибка, а не тихое использование пустых значений.
func TestLoadConfigIncompleteEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LLM_ENDPOINT", "env-endpoint")
	t.Setenv("LLM_TOKEN", "")
	t.Setenv("LLM_MODEL", "env-model")

	if _, err := LoadConfig(); err == nil {
		t.Error("LoadConfig() should fail when LLM_TOKEN is missing")
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
