package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/storage"
)

// TaskRepository — SQLite-реализация storage.TaskRepository.
type TaskRepository struct {
	db *DB
}

// NewTaskRepository создаёт репозиторий задач.
func NewTaskRepository(db *DB) *TaskRepository {
	return &TaskRepository{db: db}
}

// Save сохраняет или обновляет задачу.
func (r *TaskRepository) Save(ctx context.Context, task domain.Task) error {
	constraints, err := json.Marshal(task.Constraints)
	if err != nil {
		return fmt.Errorf("sqlite: сериализация ограничений: %w", err)
	}

	const query = `
INSERT INTO tasks (id, title, description, constraints, mode) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	title = excluded.title,
	description = excluded.description,
	constraints = excluded.constraints,
	mode = excluded.mode`
	if _, err := r.db.ExecContext(ctx, query,
		task.ID, task.Title, task.Description, string(constraints), string(task.Mode),
	); err != nil {
		return fmt.Errorf("sqlite: сохранение задачи: %w", err)
	}
	return nil
}

// Get читает задачу по идентификатору.
func (r *TaskRepository) Get(ctx context.Context, id string) (domain.Task, error) {
	var task domain.Task
	var constraints string

	const query = `SELECT id, title, description, constraints, mode FROM tasks WHERE id = ?`
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&task.ID, &task.Title, &task.Description, &constraints, &task.Mode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, storage.ErrNotFound
	}
	if err != nil {
		return domain.Task{}, fmt.Errorf("sqlite: чтение задачи: %w", err)
	}

	if err := json.Unmarshal([]byte(constraints), &task.Constraints); err != nil {
		return domain.Task{}, fmt.Errorf("sqlite: разбор ограничений: %w", err)
	}
	return task, nil
}