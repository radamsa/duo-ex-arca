// Подкоманда config: показать конфигурацию без API-ключей.
package main

import (
	"fmt"

	"github.com/radamsa/duo-ex-arca/internal/config"
)

// runConfigView выводит конфигурацию и проверяет её валидность.
func runConfigView(_ []string, cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	fmt.Println("Конфигурация:")
	fmt.Println("Участник A:")
	printParticipant(cfg.LLM.ParticipantA)
	fmt.Println("Участник B:")
	printParticipant(cfg.LLM.ParticipantB)

	fmt.Printf("Режим по умолчанию: %s\n", cfg.Debate.DefaultMode)
	fmt.Printf("Порог консенсуса:   %.2f\n", cfg.Debate.ConsensusThreshold)
	fmt.Printf("Лимиты раундов:     ")
	for _, name := range []string{"normal", "deliberate", "critical"} {
		n := cfg.Debate.MaxRounds[name]
		if n < 1 {
			n = config.Default().Debate.MaxRounds[name]
		}
		fmt.Printf("%s=%d ", name, n)
	}
	fmt.Println()

	fmt.Printf("Хранилище:          %s (%s)\n", cfg.Storage.Type, cfg.Storage.Path)
	return nil
}

// printParticipant печатает участника, не раскрывая API-ключ.
func printParticipant(p config.ParticipantConfig) {
	fmt.Printf("  base_url:   %s\n", p.BaseURL)
	fmt.Printf("  model:      %s\n", p.Model)
	if p.APIKeyEnv != "" {
		fmt.Printf("  api_key:    из $%s (не показывается)\n", p.APIKeyEnv)
	} else {
		fmt.Printf("  api_key:    не задан\n")
	}
}