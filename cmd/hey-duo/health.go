// Подкоманда health: проверить доступность обоих LLM-провайдеров.
package main

import (
	"context"
	"fmt"
	"os"
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
func runHealth(_ []string, cfg config.Config, dev bool, logPath string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	logFile, err := openLogFile(logPath)
	if err != nil {
		return err
	}
	if logFile != nil {
		defer logFile.Close()
	}

	clientA := newParticipantLLM("participant-a", cfg.LLM.ParticipantA, llmTimeout(cfg), dev, logFile, nil)
	clientB := newParticipantLLM("participant-b", cfg.LLM.ParticipantB, llmTimeout(cfg), dev, logFile, nil)

	// В режиме --dev спиннер не нужен: ход проверки виден в stderr.
	var activity *activityReporter
	if !dev {
		activity = newActivityReporter(os.Stdout, nil)
	}

	okA := healthCheck(activity, "participant-a", cfg.LLM.ParticipantA, clientA)
	okB := healthCheck(activity, "participant-b", cfg.LLM.ParticipantB, clientB)

	if !okA || !okB {
		return fmt.Errorf("health: один или оба LLM-провайдера недоступны")
	}
	return nil
}

// healthCheck пингует участника под спиннером и печатает результат.
func healthCheck(activity *activityReporter, name string, p config.ParticipantConfig, client llm.LLM) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if activity != nil {
		activity.set(name, "проверяется")
		activity.start()
	}
	resp, err := client.Generate(ctx, pingPrompt)
	if activity != nil {
		activity.stopAndWait()
	}

	if err != nil {
		fmt.Printf("%s %s: недоступен (%v)\n", name, p.Model, err)
		return false
	}
	fmt.Printf("%s %s: доступен (уверенность не оценивается, ответ: %q)\n", name, p.Model, resp.Content)
	return true
}

// intPtr возвращает указатель на int.
func intPtr(v int) *int {
	return &v
}