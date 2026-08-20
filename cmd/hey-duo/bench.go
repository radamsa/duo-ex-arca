// Подкоманда bench: прогнать JSONL-датасет через агента (TASK-150..153).
//
//	hey-duo bench --config cfg.yaml --dataset benchmark.jsonl [--mode fast|normal|...] [--limit N]
//
// baseline (fast) — одна модель без дебата; duo (normal/deliberate) — дебат.
// Каждый прогон сохраняется в SQLite (таблица benchmark_runs).
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/radamsa/duo-ex-arca/internal/agent"
	"github.com/radamsa/duo-ex-arca/internal/bench"
	"github.com/radamsa/duo-ex-arca/internal/config"
	ctxb "github.com/radamsa/duo-ex-arca/internal/context"
	"github.com/radamsa/duo-ex-arca/internal/debate"
	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/storage/sqlite"
)

// benchFlags — разобранные флаги подкоманды bench.
type benchFlags struct {
	dataset string
	mode    string
	limit   int
}

// parseBenchFlags разбирает: --dataset (обязателен), --mode, --limit.
func parseBenchFlags(args []string) (benchFlags, error) {
	var flags benchFlags
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--dataset" || a == "-dataset":
			if i+1 >= len(args) {
				return flags, fmt.Errorf("hey-duo: флаг %s требует путь к датасету", a)
			}
			flags.dataset = args[i+1]
			i++
		case strings.HasPrefix(a, "--dataset="):
			flags.dataset = strings.TrimPrefix(a, "--dataset=")
		case a == "--mode" || a == "-mode":
			if i+1 >= len(args) {
				return flags, fmt.Errorf("hey-duo: флаг %s требует значение", a)
			}
			flags.mode = args[i+1]
			i++
		case strings.HasPrefix(a, "--mode="):
			flags.mode = strings.TrimPrefix(a, "--mode=")
		case a == "--limit" || a == "-limit":
			if i+1 >= len(args) {
				return flags, fmt.Errorf("hey-duo: флаг %s требует число", a)
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				return flags, fmt.Errorf("hey-duo: невалидный --limit %q", args[i+1])
			}
			flags.limit = n
			i++
		default:
			return flags, fmt.Errorf("hey-duo: неизвестный флаг %s", a)
		}
	}
	if flags.dataset == "" {
		return flags, fmt.Errorf("hey-duo: bench требует --dataset <файл.jsonl>")
	}
	return flags, nil
}

// runBench выполняет подкоманду bench.
func runBench(args []string, cfg config.Config) error {
	flags, err := parseBenchFlags(args)
	if err != nil {
		return err
	}

	modeName := flags.mode
	if modeName == "" {
		modeName = cfg.Debate.DefaultMode
	}
	mode, ok := modeNameToMode(strings.ToLower(modeName))
	if !ok {
		return fmt.Errorf("hey-duo: невалидный режим %q (fast/normal/deliberate/critical)", modeName)
	}

	items, err := bench.LoadDataset(flags.dataset)
	if err != nil {
		return err
	}
	if flags.limit > 0 && flags.limit < len(items) {
		items = items[:flags.limit]
	}
	if len(items) == 0 {
		return fmt.Errorf("hey-duo: датасет %s пуст", flags.dataset)
	}

	// Инструментированный pipeline: токены собираются обёрткой вокруг LLM.
	counter := bench.NewTokenCounter()
	a, err := buildBenchApp(cfg, counter)
	if err != nil {
		return err
	}
	defer a.db.Close()

	benchRunner, err := bench.NewRunner(a.runner)
	if err != nil {
		return err
	}

	models := cfg.LLM.ParticipantA.Model + "|" + cfg.LLM.ParticipantB.Model
	ctx := context.Background()

	fmt.Printf("=== Бенчмарк: %s (режим %s, задач %d) ===\n", flags.dataset, mode, len(items))
	fmt.Printf("%-6s %-6s %-12s %-6s %-8s %-14s %s\n",
		"id", "score", "latency", "rounds", "tokens", "status", "решение")

	var (
		scored, correct int
		totalLatency    time.Duration
		totalTokens     int
	)
	for _, item := range items {
		result, runErr := benchRunner.RunOne(ctx, item, mode, counter)

		fmt.Printf("%-6s %-6.2f %-12s %-6d %-8d %-14s %s\n",
			item.ID, result.Score, result.Latency.Round(time.Millisecond),
			result.Rounds, result.Tokens, result.Status,
			truncateText(result.Decision, 40))
		if runErr != nil {
			fmt.Fprintf(os.Stderr, "  ошибка прогона %s: %v\n", item.ID, runErr)
		}
		if result.Status == domain.Failed {
			return runErr
		}

		if result.Score >= 0 {
			scored++
			if result.Score == 1 {
				correct++
			}
		}
		totalLatency += result.Latency
		totalTokens += result.Tokens

		result.Models = models
		if err := a.benchRepo.Save(ctx, result); err != nil {
			return err
		}
	}

	accuracy := 0.0
	if scored > 0 {
		accuracy = float64(correct) / float64(scored)
	}
	fmt.Printf("--- Итог: оценено %d/%d, меткость %.2f, средняя задержка %s, токенов %d ---\n",
		scored, len(items), accuracy,
		time.Duration(int64(totalLatency.Nanoseconds()/int64(len(items)))), totalTokens)
	return nil
}

// truncateText обрезает текст решения для вывода.
func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// buildBenchApp собирает pipeline с подсчётом токенов (TASK-153).
func buildBenchApp(cfg config.Config, counter *bench.TokenCounter) (*app, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	contextBuilder := ctxb.New()
	participantA := debate.NewParticipant("participant-a", bench.Instrument(newClient(cfg.LLM.ParticipantA, llmTimeout(cfg)), counter))
	participantB := debate.NewParticipant("participant-b", bench.Instrument(newClient(cfg.LLM.ParticipantB, llmTimeout(cfg)), counter))

	db, err := sqlite.Open(cfg.Storage.Path)
	if err != nil {
		return nil, err
	}
	closeOnError := func(e error) (*app, error) {
		_ = db.Close()
		return nil, e
	}

	traceRepo := sqlite.NewTraceRepository(db)
	recorder := newSQLiteRecorder(traceRepo)

	engine, err := debate.NewEngine(debate.EngineConfig{
		ParticipantA:       participantA,
		ParticipantB:       participantB,
		ContextBuilder:     contextBuilder,
		ConsensusThreshold: cfg.Debate.ConsensusThreshold,
		MaxRounds:          modeRounds(cfg.Debate.MaxRounds),
		Trace:              recorder,
	})
	if err != nil {
		return closeOnError(err)
	}

	runner, err := agent.NewRunner(agent.RunnerConfig{
		Engine:          engine,
		FastParticipant: participantA,
		ContextBuilder:  contextBuilder,
		Trace:           recorder,
	})
	if err != nil {
		return closeOnError(err)
	}

	return &app{
		runner:    runner,
		db:        db,
		tasks:     sqlite.NewTaskRepository(db),
		debates:   sqlite.NewDebateRepository(db),
		traces:    traceRepo,
		benchRepo: sqlite.NewBenchmarkRepository(db),
		cfg:       cfg,
	}, nil
}