// Тесты интерактивного setup: сбор конфигурации через функцию-опросчик.
// Сам ввод из stdin в unit-тестах не тестируется — проверяется логика
// сборки значений и сериализация в YAML.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/config"
	"gopkg.in/yaml.v3"
)

// scriptedAsker возвращает опросчик с заранее заданными ответами
// по порядку вопросов; пустая строка означает «значение по умолчанию»
// (по умолчания применяются в collectConfig).
func scriptedAsker(answers ...string) asker {
	calls := 0
	return func(label, defaultValue string) string {
		if calls >= len(answers) {
			return defaultValue
		}
		answer := answers[calls]
		calls++
		return answer
	}
}

// TestCollectConfigDefaults — пустые ответы дают конфигурацию по умолчанию.
func TestCollectConfigDefaults(t *testing.T) {
	cfg, err := collectConfig(scriptedAsker(
		"", "", "", // участник A: дефолты, без ключа
		"", "", "", // участник B: дефолты, без ключа
		"", "", // режим, порог
		"", "", "", // лимиты раундов
		"", // таймаут LLM
		"", // путь БД
	))
	if err != nil {
		t.Fatalf("collectConfig: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.LLM.ParticipantA.BaseURL != defaultBaseURL || cfg.LLM.ParticipantA.Model != defaultModel {
		t.Fatalf("участник A: %+v (ожидались %s/%s)", cfg.LLM.ParticipantA, defaultBaseURL, defaultModel)
	}
	if cfg.LLM.ParticipantB.APIKeyEnv != "" {
		t.Fatal("APIKeyEnv B должен быть пустым по умолчанию")
	}
	if cfg.Debate.DefaultMode != "deliberate" {
		t.Fatalf("DefaultMode = %s, ожидался deliberate", cfg.Debate.DefaultMode)
	}
	if cfg.Debate.MaxRounds["critical"] != 6 {
		t.Fatalf("MaxRounds.critical = %d, ожидалось 6", cfg.Debate.MaxRounds["critical"])
	}
	if cfg.LLM.TimeoutSeconds != 300 {
		t.Fatalf("TimeoutSeconds = %d, ожидался дефолт 300", cfg.LLM.TimeoutSeconds)
	}
	if cfg.Storage.Path != defaultDBPath {
		t.Fatalf("Storage.Path = %s, ожидался %s", cfg.Storage.Path, defaultDBPath)
	}
}

// TestCollectConfigCustom — явные ответы попадают в конфигурацию.
func TestCollectConfigCustom(t *testing.T) {
	cfg, err := collectConfig(scriptedAsker(
		"https://openrouter.ai/api/v1", "openai/gpt-4o-mini", "OPENROUTER_API_KEY",
		"https://openrouter.ai/api/v1", "anthropic/claude-3.5-haiku", "OPENROUTER_API_KEY",
		"fast", "0.7",
		"2", "4", "8",
		"600", // таймаут LLM
		"./custom.db",
	))
	if err != nil {
		t.Fatalf("collectConfig: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.LLM.ParticipantA.Model != "openai/gpt-4o-mini" {
		t.Fatalf("модель A = %s", cfg.LLM.ParticipantA.Model)
	}
	if cfg.LLM.ParticipantB.APIKeyEnv != "OPENROUTER_API_KEY" {
		t.Fatalf("APIKeyEnv B = %s", cfg.LLM.ParticipantB.APIKeyEnv)
	}
	if cfg.Debate.DefaultMode != "fast" {
		t.Fatalf("DefaultMode = %s, ожидался fast", cfg.Debate.DefaultMode)
	}
	if cfg.Debate.ConsensusThreshold != 0.7 {
		t.Fatalf("ConsensusThreshold = %v, ожидалось 0.7", cfg.Debate.ConsensusThreshold)
	}
	if cfg.Debate.MaxRounds["normal"] != 2 || cfg.Debate.MaxRounds["deliberate"] != 4 || cfg.Debate.MaxRounds["critical"] != 8 {
		t.Fatalf("лимиты раундов: %+v", cfg.Debate.MaxRounds)
	}
	if cfg.LLM.TimeoutSeconds != 600 {
		t.Fatalf("TimeoutSeconds = %d, ожидалось 600", cfg.LLM.TimeoutSeconds)
	}
	if cfg.Storage.Path != "./custom.db" {
		t.Fatalf("Storage.Path = %s", cfg.Storage.Path)
	}
}

// TestCollectConfigInvalid — невалидные ответы отклоняются.
func TestCollectConfigInvalid(t *testing.T) {
	// Невалидный режим.
	if _, err := collectConfig(scriptedAsker(
		"", "", "", "", "", "",
		"ultra", "0.85",
		"", "", "", "",
	)); err == nil {
		t.Fatal("ожидалась ошибка для невалидного режима")
	}

	// Порог вне диапазона.
	if _, err := collectConfig(scriptedAsker(
		"", "", "", "", "", "",
		"normal", "1.5",
		"", "", "", "",
	)); err == nil {
		t.Fatal("ожидалась ошибка для порога вне диапазона")
	}

	// Лимит раундов меньше 1.
	if _, err := collectConfig(scriptedAsker(
		"", "", "", "", "", "",
		"normal", "0.8",
		"0", "1", "1",
		"",
	)); err == nil {
		t.Fatal("ожидалась ошибка для лимита раундов 0")
	}
}

// TestSetupWritesValidYAML — сериализация даёт рабочий конфиг.
func TestSetupWritesValidYAML(t *testing.T) {
	cfg, err := collectConfig(scriptedAsker(
		"", "", "",
		"", "", "",
		"", "",
		"", "", "",
		"",
	))
	if err != nil {
		t.Fatalf("collectConfig: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "arca.yaml")
	writeYAML := func() error {
		data, err := yamlMarshal(&cfg)
		if err != nil {
			return err
		}
		return os.WriteFile(path, data, 0o600)
	}
	if err := writeYAML(); err != nil {
		t.Fatalf("запись: %v", err)
	}

	// Загружаем обратно через config.Load — файл должен быть валидным.
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("Validate загруженного: %v", err)
	}
	if loaded.Debate.DefaultMode != cfg.Debate.DefaultMode {
		t.Fatalf("режим после round-trip = %s", loaded.Debate.DefaultMode)
	}
	if loaded.Storage.Type != "sqlite" {
		t.Fatalf("Storage.Type после round-trip = %s", loaded.Storage.Type)
	}

	// Файл не должен содержать никаких значений ключей.
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "api_key") && strings.Contains(string(data), ": \"") {
		// api_key_env может присутствовать, но сам ключ — нет (проверяем отсутствие секрета).
		if strings.Contains(string(data), "sk-") {
			t.Fatal("в конфигурацию попал секрет")
		}
	}
}

// Привязки для тестов.
var yamlMarshal = yaml.Marshal
