// Запуск бенчмарк-прогона одной задачи через агента.
package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/radamsa/duo-ex-arca/internal/agent"
	"github.com/radamsa/duo-ex-arca/internal/domain"
)

// BenchmarkRun — результат прогона одной задачи (TASK-153).
// Сохраняется в SQLite для последующего сравнения прогонов.
type BenchmarkRun struct {
	// ItemID — id задачи из датасета.
	ItemID string
	// TaskID — id задачи в хранилище.
	TaskID string
	// Mode — режим прогона: FAST (baseline) или дебат.
	Mode domain.TaskMode
	// Models — идентификаторы моделей участников (для отчётов).
	Models string

	Rounds  int
	Latency time.Duration
	Tokens  int

	Status   domain.DecisionStatus
	Decision string
	Score    float64
}

// Runner — бенчмарк-прогонщик одной задачи.
type Runner struct {
	agent    *agent.Runner
	evaluator *Evaluator
}

// NewRunner создаёт бенчмарк-прогонщик.
func NewRunner(agentRunner *agent.Runner) (*Runner, error) {
	if agentRunner == nil {
		return nil, fmt.Errorf("bench: не задан agent runner")
	}
	return &Runner{
		agent:     agentRunner,
		evaluator: NewEvaluator(),
	}, nil
}

// RunOne выполняет одну задачу датасета и оценивает результат.
// Задача сохраняется вызывающей стороной; здесь — только прогон и оценка.
func (r *Runner) RunOne(ctx context.Context, item DatasetItem, mode domain.TaskMode, tokenCounter *TokenCounter) (BenchmarkRun, error) {
	start := time.Now()

	task, err := domain.NewTask("bench-"+item.ID+"-"+fmt.Sprint(start.UnixNano()), item.Task, "", nil, mode)
	if err != nil {
		return BenchmarkRun{}, fmt.Errorf("bench: задача: %w", err)
	}

	tokenCounter.Reset()
	decision, debate, runErr := r.agent.Run(ctx, task)

	result := BenchmarkRun{
		ItemID:   item.ID,
		TaskID:   task.ID,
		Mode:     mode,
		Latency:  time.Since(start),
		Tokens:   tokenCounter.Total(),
		Status:   decision.Status,
		Decision: decision.Decision,
		Score:    r.evaluator.Score(item.Expected, decision),
	}
	if debate != nil {
		result.Rounds = debate.RoundsCount()
	}
	if runErr != nil {
		result.Status = domain.Failed
		return result, fmt.Errorf("bench: задача %s: %w", item.ID, runErr)
	}
	return result, nil
}