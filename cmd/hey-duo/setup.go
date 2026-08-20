// Подкоманда setup: интерактивная настройка агента и создание
// конфигурационного файла (YAML).
//
// Пользователю по очереди задаются вопросы; пустой ввод принимает
// значение по умолчанию (показано в [скобках]). API-ключи в файл
// не пишутся — запрашивается только имя переменной окружения
// (инвариант: никаких ключей в конфигурации).
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/radamsa/duo-ex-arca/internal/config"
	"gopkg.in/yaml.v3"
)

// asker — функция опроса одного параметра: вопрос + значение по умолчанию.
// Возвращает введённый текст (пустой ввод = значение по умолчанию).
type asker func(label, defaultValue string) string

// setupDefaults — значения по умолчанию для мастера (локальный сервер).
const (
	defaultBaseURL   = "http://localhost:11434/v1"
	defaultModel     = "llama3.1"
	defaultDBPath    = "./arca.db"
	defaultSetupPath = "./arca.yaml"
)

// collectConfig собирает конфигурацию через опрос asker.
// Возвращает валидную конфигурацию или ошибку с подсказкой.
func collectConfig(ask asker) (config.Config, error) {
	cfg := config.Default()

	askValue := func(label, defaultValue string) string {
		value := ask(label, defaultValue)
		if strings.TrimSpace(value) == "" {
			return defaultValue
		}
		return value
	}

	fmt.Printf("Участник A (первая модель)\n")
	cfg.LLM.ParticipantA.BaseURL = askValue("  base_url", defaultBaseURL)
	cfg.LLM.ParticipantA.Model = askValue("  model", defaultModel)
	cfg.LLM.ParticipantA.APIKeyEnv = askValue("  api_key_env (имя переменной; пусто — без ключа)", "")

	fmt.Printf("Участник B (вторая модель)\n")
	cfg.LLM.ParticipantB.BaseURL = askValue("  base_url", defaultBaseURL)
	cfg.LLM.ParticipantB.Model = askValue("  model", defaultModel)
	cfg.LLM.ParticipantB.APIKeyEnv = askValue("  api_key_env (имя переменной; пусто — без ключа)", "")

	mode := askValue("Режим дебата по умолчанию (fast/normal/deliberate/critical)", cfg.Debate.DefaultMode)
	switch strings.ToLower(mode) {
	case "fast", "normal", "deliberate", "critical":
		cfg.Debate.DefaultMode = strings.ToLower(mode)
	default:
		return config.Config{}, fmt.Errorf("setup: невалидный режим %q", mode)
	}

	threshold, err := askFloat(ask, "Порог уверенности консенсуса (0..1)", fmt.Sprintf("%.2f", cfg.Debate.ConsensusThreshold))
	if err != nil {
		return config.Config{}, fmt.Errorf("setup: порог консенсуса: %w", err)
	}
	if threshold < 0 || threshold > 1 {
		return config.Config{}, fmt.Errorf("setup: порог консенсуса %v вне диапазона [0,1]", threshold)
	}
	cfg.Debate.ConsensusThreshold = threshold

	// Лимиты раундов.
	// Порядок важен: значения распределяются по именам последовательно.
	modeKeys := []struct {
		name    string
		modeKey string
	}{
		{"normal", "normal"},
		{"deliberate", "deliberate"},
		{"critical", "critical"},
	}
	for _, entry := range modeKeys {
		value, err := askInt(ask, "Лимит раундов "+entry.name, strconv.Itoa(cfg.Debate.MaxRounds[entry.modeKey]))
		if err != nil {
			return config.Config{}, fmt.Errorf("setup: лимит раундов %s: %w", entry.name, err)
		}
		if value < 1 {
			return config.Config{}, fmt.Errorf("setup: лимит раундов %s должен быть >= 1", entry.name)
		}
		cfg.Debate.MaxRounds[entry.modeKey] = value
	}

	// Таймаут запроса к LLM: медленные локальные модели отвечают минутами.
	timeoutSeconds, err := askInt(ask, "Таймаут запроса к LLM (секунды)", strconv.Itoa(config.Default().LLM.TimeoutSeconds))
	if err != nil {
		return config.Config{}, fmt.Errorf("setup: таймаут: %w", err)
	}
	if timeoutSeconds < 0 {
		return config.Config{}, fmt.Errorf("setup: таймаут не может быть отрицательным: %d", timeoutSeconds)
	}
	cfg.LLM.TimeoutSeconds = timeoutSeconds

	cfg.Storage.Type = "sqlite"
	cfg.Storage.Path = askValue("Путь к файлу базы данных", defaultDBPath)

	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// askFloat опрашивает число с плавающей точкой.
func askFloat(ask asker, label, defaultValue string) (float64, error) {
	value := ask(label, defaultValue)
	if strings.TrimSpace(value) == "" {
		value = defaultValue
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("ожидалось число, получено %q", value)
	}
	return parsed, nil
}

// askInt опрашивает целое число.
func askInt(ask asker, label, defaultValue string) (int, error) {
	value := ask(label, defaultValue)
	if strings.TrimSpace(value) == "" {
		value = defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("ожидалось целое число, получено %q", value)
	}
	return parsed, nil
}

// runSetup выполняет подкоманду setup: интерактивный опрос из stdin.
func runSetup(args []string, cfgPath string) error {
	// Позиционные аргументы не принимаем — только переданный --config.
	if len(args) > 0 {
		return fmt.Errorf("hey-duo: setup не принимает аргументов (путь задаётся через --config)")
	}
	if cfgPath == "" {
		cfgPath = os.Getenv("ARCA_CONFIG")
	}
	if cfgPath == "" {
		cfgPath = defaultSetupPath
	}

	fmt.Println("Настройка Duo ex Arca")
	fmt.Println("Пустой ввод = значение по умолчанию (в [скобках]).")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	cfg, err := collectConfig(func(label, defaultValue string) string {
		if defaultValue == "" {
			fmt.Printf("%s: ", label)
		} else {
			fmt.Printf("%s [%s]: ", label, defaultValue)
		}
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return defaultValue
		}
		return line
	})
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("hey-duo: сериализация конфигурации: %w", err)
	}
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		return fmt.Errorf("hey-duo: запись конфигурации: %w", err)
	}

	fmt.Printf("\nКонфигурация записана: %s\n", cfgPath)
	fmt.Printf("Использование: hey-duo health --config %s\n", cfgPath)
	return nil
}
