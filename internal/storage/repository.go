// Пакет storage — контракты репозиториев.
//
// Слоение (docs/design.md, разделы 3 и 19):
//
//	Domain -> Repository interfaces -> SQLite implementation
//
// Domain не знает SQLite: пакет storage определяет интерфейсы,
// а internal/storage/sqlite реализует их (TASK-086).
package storage

import (
	"context"
	"errors"

	"github.com/radamsa/duo-ex-arca/internal/bench"
	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/trace"
)

// ErrNotFound — сущность не найдена.
var ErrNotFound = errors.New("storage: сущность не найдена")

// TaskRepository — хранилище задач (TASK-081).
type TaskRepository interface {
	Save(ctx context.Context, task domain.Task) error
	Get(ctx context.Context, id string) (domain.Task, error)
}

// DebateRepository — хранилище дебатов с раундами и решением (TASK-082..084).
type DebateRepository interface {
	Save(ctx context.Context, debate domain.Debate) error
	Get(ctx context.Context, id string) (domain.Debate, error)
	ListByTask(ctx context.Context, taskID string) ([]domain.Debate, error)
}

// TraceRepository — хранилище событий трассировки (TASK-085, TASK-091).
type TraceRepository interface {
	Append(ctx context.Context, event trace.Event) error
	ListByTask(ctx context.Context, taskID string) ([]trace.Event, error)
}

// BenchmarkRepository — хранилище результатов бенчмарк-прогонов (TASK-153).
type BenchmarkRepository interface {
	Save(ctx context.Context, run bench.BenchmarkRun) error
}