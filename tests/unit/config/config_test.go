// Тесты конфигурации: загрузка YAML/JSON, валидация, api_key_env.
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/config"
)

// writeFile создаёт временный файл конфигурации.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("не удалось записать конфиг: %v", err)
	}
	return path
}

const validYAML = `
llm:
  participant_a:
    base_url: "https://openrouter.ai/api/v1"
    model: "anthropic/claude-3.5-sonnet"
    api_key_env: "OPENROUTER_API_KEY"
  participant_b:
    base_url: "http://localhost:11434/v1"
    model: "llama3"
    api_key_env: ""
debate:
  default_mode: deliberate
  max_rounds:
    normal: 1
    deliberate: 3
    critical: 6
  consensus_threshold: 0.85
storage:
  type: sqlite
  path: "./arca.db"
`

// TestLoadYAML — конфиг загружается из YAML-файла.
func TestLoadYAML(t *testing.T) {
	cfg, err := config.Load(writeFile(t, validYAML))
	if err != nil {
		t.Fatalf("Load вернул ошибку: %v", err)
	}

	if cfg.LLM.ParticipantA.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("participant_a base_url = %q", cfg.LLM.ParticipantA.BaseURL)
	}
	if cfg.LLM.ParticipantA.APIKeyEnv != "OPENROUTER_API_KEY" {
		t.Fatalf("api_key_env = %q", cfg.LLM.ParticipantA.APIKeyEnv)
	}
	if cfg.LLM.ParticipantB.Model != "llama3" {
		t.Fatalf("participant_b model = %q", cfg.LLM.ParticipantB.Model)
	}
	if cfg.Debate.DefaultMode != "deliberate" || cfg.Debate.ConsensusThreshold != 0.85 {
		t.Fatalf("debate = %+v", cfg.Debate)
	}
	if cfg.Debate.MaxRounds["critical"] != 6 {
		t.Fatalf("max_rounds.critical = %d", cfg.Debate.MaxRounds["critical"])
	}
	if cfg.Storage.Type != "sqlite" || cfg.Storage.Path != "./arca.db" {
		t.Fatalf("storage = %+v", cfg.Storage)
	}
}

// TestLoadJSON — конфиг загружается из JSON-файла.
func TestLoadJSON(t *testing.T) {
	json := `{
		"llm": {
			"participant_a": {"base_url": "http://localhost:1234/v1", "model": "qwen2.5"},
			"participant_b": {"base_url": "http://localhost:11434/v1", "model": "llama3"},
			"timeout_seconds": 600
		},
		"debate": {"default_mode": "normal", "max_rounds": {"normal": 1}, "consensus_threshold": 0.8},
		"storage": {"type": "sqlite", "path": "./arca.db"}
	}`
	cfg, err := config.Load(writeFile(t, json))
	if err != nil {
		t.Fatalf("Load вернул ошибку: %v", err)
	}
	if cfg.LLM.ParticipantA.Model != "qwen2.5" || cfg.Debate.DefaultMode != "normal" {
		t.Fatalf("JSON-конфиг загружен неверно: %+v", cfg)
	}
	if cfg.LLM.TimeoutSeconds != 600 {
		t.Fatalf("timeout_seconds = %d, ожидалось 600", cfg.LLM.TimeoutSeconds)
	}
}

// TestLoadTimeoutDefault — отсутствие timeout_seconds даёт дефолт 300.
func TestLoadTimeoutDefault(t *testing.T) {
	cfg, err := config.Load(writeFile(t, validYAML))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.TimeoutSeconds != 0 {
		t.Fatalf("timeout_seconds в файле должен быть 0, получено %d", cfg.LLM.TimeoutSeconds)
	}
	if got := config.Default().LLM.TimeoutSeconds; got != 300 {
		t.Fatalf("дефолтный timeout_seconds = %d, ожидалось 300", got)
	}
}

// TestValidateNegativeTimeout — отрицательный таймаут отклоняется.
func TestValidateNegativeTimeout(t *testing.T) {
	cfg, err := config.Load(writeFile(t, validYAML))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.LLM.TimeoutSeconds = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("ожидалась ошибка для отрицательного timeout_seconds")
	}
}

// TestLoadMissingFile — отсутствующий файл — ошибка.
func TestLoadMissingFile(t *testing.T) {
	if _, err := config.Load("/нет/такого/файла.yaml"); err == nil {
		t.Fatal("ожидалась ошибка для отсутствующего файла")
	}
}

// TestLoadInvalidYAML — битый YAML — ошибка.
func TestLoadInvalidYAML(t *testing.T) {
	if _, err := config.Load(writeFile(t, "llm: [незакрытая структура")); err == nil {
		t.Fatal("ожидалась ошибка для битого YAML")
	}
}

// TestValidateValid — валидный конфиг проходит проверку.
func TestValidateValid(t *testing.T) {
	cfg, err := config.Load(writeFile(t, validYAML))
	if err != nil {
		t.Fatalf("Load вернул ошибку: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate вернул ошибку: %v", err)
	}
}

// TestValidateInvalid — ошибки валидации по каждому полю.
func TestValidateInvalid(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"пустой base_url участника A", `
llm:
  participant_a:
    base_url: ""
    model: "m-a"
  participant_b:
    base_url: "http://localhost:1/v1"
    model: "m-b"
`},
		{"пустой model участника B", `
llm:
  participant_a:
    base_url: "http://localhost:1/v1"
    model: "m-a"
  participant_b:
    base_url: "http://localhost:1/v1"
    model: ""
`},
		{"невалидный default_mode", `
llm:
  participant_a: {base_url: "http://localhost:1/v1", model: "m-a"}
  participant_b: {base_url: "http://localhost:1/v1", model: "m-b"}
debate:
  default_mode: "turbo"
`},
		{"threshold вне диапазона", `
llm:
  participant_a: {base_url: "http://localhost:1/v1", model: "m-a"}
  participant_b: {base_url: "http://localhost:1/v1", model: "m-b"}
debate:
  consensus_threshold: 1.5
`},
		{"similarity_threshold вне диапазона", `
llm:
  participant_a: {base_url: "http://localhost:1/v1", model: "m-a"}
  participant_b: {base_url: "http://localhost:1/v1", model: "m-b"}
debate:
  similarity_threshold: -0.5
`},
		{"неизвестный тип storage", `
llm:
  participant_a: {base_url: "http://localhost:1/v1", model: "m-a"}
  participant_b: {base_url: "http://localhost:1/v1", model: "m-b"}
storage:
  type: "cassandra"
  path: "./db"
`},
		{"пустой путь storage", `
llm:
  participant_a: {base_url: "http://localhost:1/v1", model: "m-a"}
  participant_b: {base_url: "http://localhost:1/v1", model: "m-b"}
storage:
  type: "sqlite"
  path: ""
`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := config.Load(writeFile(t, tc.content))
			if err != nil {
				t.Fatalf("Load вернул ошибку: %v", err)
			}
			if err := cfg.Validate(); err == nil {
				t.Error("ожидалась ошибка валидации")
			}
		})
	}
}

// TestAPIKeyFromEnv — ключ читается из переменной окружения, не из файла.
func TestAPIKeyFromEnv(t *testing.T) {
	t.Setenv("ARCA_TEST_KEY", "secret-значение")

	cfg, err := config.Load(writeFile(t, validYAML))
	if err != nil {
		t.Fatalf("Load вернул ошибку: %v", err)
	}

	p := config.ParticipantConfig{APIKeyEnv: "ARCA_TEST_KEY"}
	if got := p.APIKey(); got != "secret-значение" {
		t.Fatalf("APIKey() = %q", got)
	}

	// Ключ никогда не должен попадать в конфиг как значение.
	if cfg.LLM.ParticipantA.APIKey() != "" {
		t.Fatal("APIKey участника A должен читаться из окружения, а не из файла")
	}
	t.Setenv("OPENROUTER_API_KEY", "env-ключ")
	if got := cfg.LLM.ParticipantA.APIKey(); got != "env-ключ" {
		t.Fatalf("APIKey() участника A = %q", got)
	}
}

// TestAPIKeyUnset — пустая переменная окружения даёт пустой ключ.
func TestAPIKeyUnset(t *testing.T) {
	t.Setenv("ARCA_ABSENT_KEY", "")
	p := config.ParticipantConfig{APIKeyEnv: "ARCA_ABSENT_KEY"}
	if got := p.APIKey(); got != "" {
		t.Fatalf("APIKey() = %q, ожидалась пустая строка", got)
	}
}

// TestDefault — значения по умолчанию разумны и валидны после заполнения провайдеров.
func TestDefault(t *testing.T) {
	cfg := config.Default()
	if cfg.Debate.DefaultMode == "" || cfg.Debate.ConsensusThreshold <= 0 {
		t.Fatalf("дефолтные значения не заполнены: %+v", cfg.Debate)
	}
	if cfg.Debate.SimilarityThreshold <= 0 || cfg.Debate.SimilarityThreshold > 1 {
		t.Fatalf("дефолтный similarity_threshold не заполнен: %+v", cfg.Debate)
	}
	if cfg.Storage.Type != "sqlite" || cfg.Storage.Path == "" {
		t.Fatalf("дефолтный storage неверный: %+v", cfg.Storage)
	}
}