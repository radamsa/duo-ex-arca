package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/radamsa/duo-ex-arca/internal/trace"
)

// TraceRepository — SQLite-реализация storage.TraceRepository.
type TraceRepository struct {
	db *DB
}

// NewTraceRepository создаёт репозиторий трассировки.
func NewTraceRepository(db *DB) *TraceRepository {
	return &TraceRepository{db: db}
}

// Append сохраняет событие трассировки.
func (r *TraceRepository) Append(ctx context.Context, event trace.Event) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("sqlite: сериализация метаданных события: %w", err)
	}

	const query = `
INSERT INTO traces (trace_id, task_id, ts, event_type, participant, duration_ms, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?)`
	if _, err := r.db.ExecContext(ctx, query,
		event.TraceID, event.TaskID,
		event.Timestamp.UnixMilli(),
		string(event.Type), event.Participant,
		event.Duration.Milliseconds(),
		string(metadata),
	); err != nil {
		return fmt.Errorf("sqlite: сохранение события: %w", err)
	}
	return nil
}

// ListByTask возвращает события задачи в хронологическом порядке.
func (r *TraceRepository) ListByTask(ctx context.Context, taskID string) ([]trace.Event, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT trace_id, task_id, ts, event_type, participant, duration_ms, metadata
FROM traces WHERE task_id = ? ORDER BY ts, rowid`, taskID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: чтение событий: %w", err)
	}
	defer rows.Close()

	var events []trace.Event
	for rows.Next() {
		var ev trace.Event
		var tsMillis int64
		var durationMillis int64
		var metadata string
		var eventType string

		if err := rows.Scan(&ev.TraceID, &ev.TaskID, &tsMillis, &eventType, &ev.Participant, &durationMillis, &metadata); err != nil {
			return nil, fmt.Errorf("sqlite: чтение строки события: %w", err)
		}

		ev.Timestamp = time.UnixMilli(tsMillis).UTC()
		ev.Type = trace.EventType(eventType)
		ev.Duration = time.Duration(durationMillis) * time.Millisecond
		if err := json.Unmarshal([]byte(metadata), &ev.Metadata); err != nil {
			return nil, fmt.Errorf("sqlite: разбор метаданных события: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: чтение событий: %w", err)
	}
	return events, nil
}