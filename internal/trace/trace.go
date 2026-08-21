// Пакет trace — модель и запись событий трассировки.
//
// Каждое существенное решение имеет trace (инвариант I8,
// docs/plan-mvp.md, TASK-090..091).
package trace

import (
	"fmt"
	"time"
)

// EventType — тип события трассировки.
// Перечень событий задан в docs/plan-mvp.md, TASK-090.
type EventType string

const (
	// TaskCreated — задача создана.
	TaskCreated EventType = "TASK_CREATED"
	// ContextBuilt — контекст построен.
	ContextBuilt EventType = "CONTEXT_BUILT"
	// ProposalStarted — началась генерация initial proposals.
	ProposalStarted EventType = "PROPOSAL_STARTED"
	// ProposalCompleted — initial proposals получены.
	ProposalCompleted EventType = "PROPOSAL_COMPLETED"
	// CritiqueStarted — началась фаза взаимной критики.
	CritiqueStarted EventType = "CRITIQUE_STARTED"
	// CritiqueCompleted — критика получена.
	CritiqueCompleted EventType = "CRITIQUE_COMPLETED"
	// RevisionCompleted — пересмотры получены.
	RevisionCompleted EventType = "REVISION_COMPLETED"
	// ConsensusEvaluated — консенсус оценён.
	ConsensusEvaluated EventType = "CONSENSUS_EVALUATED"
	// SimilarityEvaluated — арбитры оценили смысловое совпадение решений.
	SimilarityEvaluated EventType = "SIMILARITY_EVALUATED"
	// DecisionCreated — итоговое решение создано.
	DecisionCreated EventType = "DECISION_CREATED"
)

// Valid возвращает true, если тип события поддерживается.
func (t EventType) Valid() bool {
	switch t {
	case TaskCreated, ContextBuilt,
		ProposalStarted, ProposalCompleted,
		CritiqueStarted, CritiqueCompleted,
		RevisionCompleted, ConsensusEvaluated, SimilarityEvaluated, DecisionCreated:
		return true
	default:
		return false
	}
}

// Event — одно событие трассировки.
type Event struct {
	TraceID string
	TaskID  string

	Timestamp time.Time

	Type EventType

	// Participant — участник, с которым связано событие (может быть пустым).
	Participant string

	// Duration — длительность фазы.
	Duration time.Duration

	// Metadata — дополнительные сведения (agreement, ключи и т.п.).
	Metadata map[string]string
}

// NewEvent создаёт событие с заполненным timestamp.
func NewEvent(traceID, taskID string, eventType EventType) (Event, error) {
	ev := Event{
		TraceID:   traceID,
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
		Type:      eventType,
	}
	if err := ev.Validate(); err != nil {
		return Event{}, err
	}
	return ev, nil
}

// Validate проверяет обязательные поля события.
func (e Event) Validate() error {
	if e.TraceID == "" {
		return fmt.Errorf("trace: пустой trace id")
	}
	if e.TaskID == "" {
		return fmt.Errorf("trace: пустой task id")
	}
	if !e.Type.Valid() {
		return fmt.Errorf("trace: невалидный тип события %q", e.Type)
	}
	return nil
}

// Recorder — приёмник событий трассировки.
type Recorder interface {
	Record(event Event) error
}

// Noop — рекордер-заглушка (когда трассировка не настроена).
type Noop struct{}

// Record игнорирует событие.
func (Noop) Record(Event) error {
	return nil
}