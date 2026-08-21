// Подкоманда ask: создать задачу, выполнить pipeline, показать решение.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/radamsa/duo-ex-arca/internal/config"
	"github.com/radamsa/duo-ex-arca/internal/domain"
)

// askFlags — разобранные флаги подкоманды ask.
type askFlags struct {
	mode     string
	asJSON   bool
	question string
}

// parseAskFlags разбирает аргументы: --mode, --json и один вопрос.
func parseAskFlags(args []string) (askFlags, error) {
	var flags askFlags
	var positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--mode" || a == "-mode":
			if i+1 >= len(args) {
				return flags, fmt.Errorf("hey-duo: флаг %s требует значение", a)
			}
			flags.mode = args[i+1]
			i++
		case strings.HasPrefix(a, "--mode="):
			flags.mode = strings.TrimPrefix(a, "--mode=")
		case a == "--json":
			flags.asJSON = true
		case strings.HasPrefix(a, "-"):
			return flags, fmt.Errorf("hey-duo: неизвестный флаг %s", a)
		default:
			positional = append(positional, a)
		}
	}

	if len(positional) != 1 {
		return flags, fmt.Errorf("hey-duo: ожидался ровно один вопрос (получено аргументов: %d)", len(positional))
	}
	flags.question = positional[0]
	return flags, nil
}

// runAsk выполняет подкоманду ask.
func runAsk(args []string, cfg config.Config, dev bool) error {
	flags, err := parseAskFlags(args)
	if err != nil {
		return err
	}
	if flags.question == "" {
		return fmt.Errorf("hey-duo: пустой вопрос")
	}

	modeName := cfg.Debate.DefaultMode
	if flags.mode != "" {
		modeName = flags.mode
	}
	mode, ok := modeNameToMode(strings.ToLower(modeName))
	if !ok {
		return fmt.Errorf("hey-duo: невалидный режим %q (fast/normal/deliberate/critical)", modeName)
	}

	a, err := buildApp(cfg, dev)
	if err != nil {
		return err
	}
	defer a.db.Close()

	ctx := context.Background()
	task, err := domain.NewTask("task-"+strconv.FormatInt(time.Now().UnixNano(), 10), flags.question, "", nil, mode)
	if err != nil {
		return err
	}
	if err := a.tasks.Save(ctx, task); err != nil {
		return err
	}

	// Спиннер активности на время прогона; останавливаем до печати
	// результата, чтобы не смешивать вывод.
	a.activity.start()
	decision, debate, runErr := a.runner.Run(ctx, task)
	a.activity.stopAndWait()

	if debate != nil {
		if err := a.debates.Save(ctx, *debate); err != nil {
			return err
		}
	}
	if runErr != nil {
		return runErr
	}

	if flags.asJSON {
		return printDecisionJSON(task, decision)
	}
	printDecisionText(task, decision)
	return nil
}

// printDecisionJSON выводит решение в JSON.
func printDecisionJSON(task domain.Task, decision domain.Decision) error {
	out := struct {
		TaskID   string            `json:"task_id"`
		Mode     domain.TaskMode   `json:"mode"`
		Decision domain.Decision   `json:"decision"`
	}{
		TaskID: task.ID,
		Mode:   task.Mode,
		Decision: decision,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("hey-duo: сериализация решения: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// printDecisionText выводит решение в человекочитаемом виде.
func printDecisionText(task domain.Task, decision domain.Decision) {
	fmt.Printf("Задача:      %s\n", task.ID)
	fmt.Printf("Режим:       %s\n", task.Mode)
	fmt.Printf("Статус:      %s\n", decision.Status)
	if decision.Decision != "" {
		fmt.Printf("Решение:     %s\n", decision.Decision)
	}
	fmt.Printf("Уверенность: %.2f\n", decision.Confidence)
	for _, arg := range decision.SupportingArguments {
		fmt.Printf("  аргумент:  %s\n", arg)
	}
	for _, risk := range decision.Risks {
		fmt.Printf("  риск:      %s\n", risk)
	}
	if decision.Status != domain.Consensus {
		fmt.Println("Единого решения нет — статус штатный для данного исхода дебата.")
	}
}