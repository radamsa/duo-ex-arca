// Подкоманда health: проверить доступность обоих LLM-провайдеров.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/radamsa/duo-ex-arca/internal/config"
	"github.com/radamsa/duo-ex-arca/internal/llm"
)

// pingPrompt — минимальный запрос для проверки доступности модели.
var pingPrompt = llm.GenerationRequest{
	Messages: []llm.Message{
		{Role: llm.RoleUser, Content: "Ответь одним словом: ок."},
	},
	MaxTokens: intPtr(8),
}

// runHealth проверяет доступность обоих участников и возвращает ошибку,
// если хотя бы один недоступен.
func runHealth(_ []string, cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	clientA := newClient(cfg.LLM.ParticipantA)
	clientB := newClient(cfg.LLM.ParticipantB)

	ok := true
	report("participant-a", cfg.LLM.ParticipantA, clientA, &ok)
	report("participant-b", cfg.LLM.ParticipantB, clientB, &ok)

	if !ok {
		return fmt.Errorf("health: один или оба LLM-провайдера недоступны")
	}
	return nil
}

// report проверяет одного участника и печатает результат.
func report(name string, p config.ParticipantConfig, client *llm.Client, ok *bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Generate(ctx, pingPrompt)
	if err != nil {
		*ok = false
		fmt.Printf("%s %s: недоступен (%v)\n", name, p.Model, err)
		return
	}
	fmt.Printf("%s %s: доступен (уверенность не оценивается, ответ: %q)\n", name, p.Model, resp.Content)
}

// intPtr возвращает указатель на int.
func intPtr(v int) *int {
	return &v
}