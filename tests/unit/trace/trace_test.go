// Тесты модели trace-событий.
package trace_test

import (
	"testing"
	"time"

	"github.com/radamsa/duo-ex-arca/internal/trace"
)

// TestEventTypesValid — все события из плана валидны.
func TestEventTypesValid(t *testing.T) {
	valid := []trace.EventType{
		trace.TaskCreated,
		trace.ContextBuilt,
		trace.ProposalStarted,
		trace.ProposalCompleted,
		trace.CritiqueStarted,
		trace.CritiqueCompleted,
		trace.RevisionCompleted,
		trace.ConsensusEvaluated,
		trace.DecisionCreated,
	}
	for _, e := range valid {
		if !e.Valid() {
			t.Errorf("%q должен быть валидным типом события", e)
		}
	}
	if (trace.EventType("UNKNOWN")).Valid() {
		t.Error("неизвестный тип события не должен быть валидным")
	}
}

// TestNewEventValid — событие с корректными полями.
func TestNewEventValid(t *testing.T) {
	ev, err := trace.NewEvent("trace-1", "task-1", trace.ProposalCompleted)
	if err != nil {
		t.Fatalf("NewEvent вернул ошибку: %v", err)
	}
	if ev.TraceID != "trace-1" || ev.TaskID != "task-1" || ev.Type != trace.ProposalCompleted {
		t.Fatalf("поля события неверны: %+v", ev)
	}
	if ev.Timestamp.IsZero() {
		t.Fatal("timestamp не заполнен")
	}
}

// TestNewEventInvalid — пустые идентификаторы отклоняются.
func TestNewEventInvalid(t *testing.T) {
	cases := []struct {
		name  string
		trace string
		task  string
		typ   trace.EventType
	}{
		{"пустой trace id", "", "task-1", trace.TaskCreated},
		{"пустой task id", "trace-1", "", trace.TaskCreated},
		{"неизвестный тип", "trace-1", "task-1", "MAGIC_EVENT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := trace.NewEvent(tc.trace, tc.task, tc.typ); err == nil {
				t.Error("ожидалась ошибка валидации")
			}
		})
	}
}

// TestEventWithMetadata — метаданные и длительность сохраняются.
func TestEventWithMetadata(t *testing.T) {
	ev, _ := trace.NewEvent("trace-1", "task-1", trace.ConsensusEvaluated)
	ev.Duration = 250 * time.Millisecond
	ev.Metadata = map[string]string{"agreement": "CONSENSUS"}

	if err := ev.Validate(); err != nil {
		t.Fatalf("валидное событие не прошло проверку: %v", err)
	}
}

// TestNoopRecorder — рекордер-заглушка не возвращает ошибок.
func TestNoopRecorder(t *testing.T) {
	rec := trace.Noop{}
	ev, _ := trace.NewEvent("trace-1", "task-1", trace.TaskCreated)
	if err := rec.Record(ev); err != nil {
		t.Fatalf("Noop.Record вернул ошибку: %v", err)
	}
}