package sqlite

import (
	"context"
	"fmt"

	"github.com/radamsa/duo-ex-arca/internal/bench"
)

// BenchmarkRepository — SQLite-реализация storage.BenchmarkRepository (TASK-153).
type BenchmarkRepository struct {
	db *DB
}

// NewBenchmarkRepository создаёт репозиторий бенчмарков.
func NewBenchmarkRepository(db *DB) *BenchmarkRepository {
	return &BenchmarkRepository{db: db}
}

// Save сохраняет результат одного бенчмарк-прогона.
func (r *BenchmarkRepository) Save(ctx context.Context, run bench.BenchmarkRun) error {
	const query = `
INSERT INTO benchmark_runs (
	item_id, task_id, mode, models, rounds, latency_ms, tokens, status, decision, score
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := r.db.ExecContext(ctx, query,
		run.ItemID, run.TaskID, string(run.Mode), run.Models,
		run.Rounds, run.Latency.Milliseconds(), run.Tokens,
		string(run.Status), run.Decision, run.Score,
	); err != nil {
		return fmt.Errorf("sqlite: сохранение бенчмарк-прогона: %w", err)
	}
	return nil
}