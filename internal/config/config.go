// Пакет config — конфигурация агента (docs/plan-mvp.md, TASK-020..022).
//
// Конфигурация загружается из YAML или JSON файла (YAML — надмножество JSON).
// API-ключи в файл не кладутся: для каждого участника задаётся имя
// переменной окружения api_key_env (TASK-022).
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config — корневая конфигурация агента.
type Config struct {
	LLM     LLMConfig     `yaml:"llm"`
	Debate  DebateConfig  `yaml:"debate"`
	Storage StorageConfig `yaml:"storage"`
}

// LLMConfig — конфигурация двух участников дебата.
type LLMConfig struct {
	ParticipantA ParticipantConfig `yaml:"participant_a"`
	ParticipantB ParticipantConfig `yaml:"participant_b"`

	// TimeoutSeconds — таймаут одного HTTP-запроса к LLM (секунды).
	// 0 означает значение по умолчанию (см. Default).
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

// ParticipantConfig — настройки отдельного LLM-провайдера.
type ParticipantConfig struct {
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
	// APIKeyEnv — имя переменной окружения с API-ключом.
	// Сам ключ в конфигурации не хранится.
	APIKeyEnv string `yaml:"api_key_env"`
}

// APIKey читает ключ участника из переменной окружения.
func (p ParticipantConfig) APIKey() string {
	return os.Getenv(p.APIKeyEnv)
}

// DebateConfig — настройки протокола дебата.
type DebateConfig struct {
	DefaultMode string `yaml:"default_mode"`

	// MaxRounds — лимит раундов по режимам: normal/deliberate/critical.
	MaxRounds map[string]int `yaml:"max_rounds"`

	// ConsensusThreshold — порог уверенности консенсуса [0,1].
	ConsensusThreshold float64 `yaml:"consensus_threshold"`
}

// StorageConfig — настройки хранилища.
type StorageConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

// Load читает конфигурацию из файла (YAML или JSON).
func Load(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: не удалось прочитать файл %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: невалидный YAML/JSON в %s: %w", path, err)
	}
	return cfg, nil
}

// defaultTimeoutSeconds — таймаут запроса к LLM по умолчанию (5 минут):
// локальные и медленные модели могут отвечать несколько минут.
const defaultTimeoutSeconds = 300

// Validate проверяет обязательные поля конфигурации.
func (c Config) Validate() error {
	if c.LLM.ParticipantA.BaseURL == "" {
		return fmt.Errorf("config: participant_a.base_url обязателен")
	}
	if c.LLM.ParticipantA.Model == "" {
		return fmt.Errorf("config: participant_a.model обязателен")
	}
	if c.LLM.ParticipantB.BaseURL == "" {
		return fmt.Errorf("config: participant_b.base_url обязателен")
	}
	if c.LLM.ParticipantB.Model == "" {
		return fmt.Errorf("config: participant_b.model обязателен")
	}
	if c.LLM.TimeoutSeconds < 0 {
		return fmt.Errorf("config: timeout_seconds не может быть отрицательным: %d", c.LLM.TimeoutSeconds)
	}
	if !validMode(c.Debate.DefaultMode) {
		return fmt.Errorf("config: невалидный debate.default_mode %q", c.Debate.DefaultMode)
	}
	if c.Debate.ConsensusThreshold < 0 || c.Debate.ConsensusThreshold > 1 {
		return fmt.Errorf("config: consensus_threshold %v вне диапазона [0,1]", c.Debate.ConsensusThreshold)
	}
	if c.Storage.Type != "sqlite" {
		return fmt.Errorf("config: поддерживается только storage.type=sqlite, получено %q", c.Storage.Type)
	}
	if c.Storage.Path == "" {
		return fmt.Errorf("config: storage.path обязателен")
	}
	return nil
}

// validMode проверяет режим дебата по умолчанию.
func validMode(mode string) bool {
	switch mode {
	case "fast", "normal", "deliberate", "critical":
		return true
	default:
		return false
	}
}

// Default возвращает конфигурацию с разумными значениями по умолчанию.
// Провайдеры LLM не заполняются — они обязаны прийти из файла.
func Default() Config {
	return Config{
		LLM: LLMConfig{
			TimeoutSeconds: defaultTimeoutSeconds,
		},
		Debate: DebateConfig{
			DefaultMode: "deliberate",
			MaxRounds: map[string]int{
				"normal":    1,
				"deliberate": 3,
				"critical":   6,
			},
			ConsensusThreshold: 0.85,
		},
		Storage: StorageConfig{
			Type: "sqlite",
			Path: "./arca.db",
		},
	}
}