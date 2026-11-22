package ai

import (
	"errors"

	"dev/internal/i18n"
	"dev/internal/selfconfig"
)

const (
	// UserConfigPath — пользовательский конфиг (высший приоритет).
	UserConfigPath = selfconfig.UserPath
	// EtcConfigPath — системный конфиг (используется, если пользовательского нет).
	EtcConfigPath = selfconfig.EtcPath
)

// Config хранит параметры подключения к LLM и язык интерфейса.
type Config struct {
	Endpoint string
	Token    string
	Model    string
	Language string
}

// ConfigPaths возвращает пути к конфигу в порядке приоритета.
func ConfigPaths() []string {
	return selfconfig.Paths()
}

// configParam связывает ключ конфига с полем структуры Config.
type configParam struct {
	key   string
	field func(*Config) *string
}

// configParams — единственное место, где перечислены параметры конфига.
// По этой таблице и читается файл, и накладывается окружение, поэтому
// отдельного списка переменных окружения не существует.
var configParams = []configParam{
	{key: selfconfig.KeyLLMEndpoint, field: func(c *Config) *string { return &c.Endpoint }},
	{key: selfconfig.KeyLLMToken, field: func(c *Config) *string { return &c.Token }},
	{key: selfconfig.KeyLLMModel, field: func(c *Config) *string { return &c.Model }},
	{key: selfconfig.KeyInterfaceLang, field: func(c *Config) *string { return &c.Language }},
}

// applyValues переносит разрешённые значения в поля структуры.
func (c *Config) applyValues(values map[string]string) {
	for _, p := range configParams {
		if v, ok := values[p.key]; ok {
			*p.field(c) = v
		}
	}
}

// complete сообщает, что заданы все обязательные параметры подключения.
func (c *Config) complete() bool {
	return c.Endpoint != "" && c.Token != "" && c.Model != ""
}

// LoadConfig загружает параметры конфига. Значения берутся из файла
// (~/dev-command/main.conf, затем /etc/dev-command/main.conf) и окружения:
// переменная с именем ключа приоритетнее файла.
func LoadConfig() (*Config, error) {
	for _, path := range selfconfig.Paths() {
		fileValues, err := selfconfig.ReadFile(path)
		if err != nil {
			continue
		}

		cfg := &Config{}
		cfg.applyValues(selfconfig.ApplyEnv(fileValues))
		if cfg.complete() {
			return cfg, nil
		}
	}

	// Ни один файл не дал полного набора: параметры могут быть заданы
	// окружением (CI).
	cfg := &Config{}
	cfg.applyValues(selfconfig.ApplyEnv(nil))
	if cfg.complete() {
		return cfg, nil
	}

	return nil, errors.New(i18n.T("incomplete config"))
}

// LanguageFromConfig возвращает язык интерфейса из параметров конфига.
func LanguageFromConfig() string {
	return selfconfig.Language()
}

// EditConfig открывает конфиг на редактирование, создавая при необходимости.
// Создаёт/редактирует ~/dev-command/main.conf (пользовательский путь).
func EditConfig() error {
	return selfconfig.Edit()
}

// InteractiveEditConfig спрашивает пользователя и открывает редактор.
func InteractiveEditConfig() error {
	return selfconfig.InteractiveEdit()
}
