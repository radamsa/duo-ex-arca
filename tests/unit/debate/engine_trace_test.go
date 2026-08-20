// Тесты трассировки событий Debate Engine (TASK-090/091).
package debate_test

import (
	"context"
	"sync"
	"testing"

	ctxb "github.com/radamsa/duo-ex-arca/internal/context"
	"github.com/radamsa/duo-ex-arca/internal/debate"
	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/llm"
	"github.com/radamsa/duo-ex-arca/internal/trace"
)

// memRecorder — рекордер трассировки в памяти для тестов.
type memRecorder struct {
	mu     sync.Mutex
	events []trace.Event
}

func (m *memRecorder) Record(ev trace.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, ev)
	return nil
}

func (m *memRecorder) types() []trace.EventType {
	m.mu.Lock()
	defer m.mu.Unlock()
	var types []trace.EventType
	for _, ev := range m.events {
		types = append(types, ev.Type)
	}
	return types
}

// TestEngineEmitsTraceEvents — движок пишет события всех фаз.
func TestEngineEmitsTraceEvents(t *testing.T) {
	mockA := llm.NewMock()
	mockB := llm.NewMock()
	rec := &memRecorder{}

	engine, err := debate.NewEngine(debate.EngineConfig{
		ParticipantA:      debate.NewParticipant("participant-a", mockA),
		ParticipantB:      debate.NewParticipant("participant-b", mockB),
		ContextBuilder:    ctxb.New(),
		ConsensusThreshold: 0.8,
		MaxRounds: map[domain.TaskMode]int{
			domain.NORMAL: 1, domain.DELIBERATE: 3, domain.CRITICAL: 6,
		},
		Trace: rec,
	})
	if err != nil {
		t.Fatal(err)
	}

	scriptConsensusRound(mockA, mockB,
		verdictJSON("CONSENSUS", "SQLite", 0.9),
		verdictJSON("CONSENSUS", "SQLite", 0.9))

	task, err := domain.NewTask("t-1", "Заголовок", "", nil, domain.NORMAL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Deliberate(context.Background(), task, "run-1"); err != nil {
		t.Fatalf("Deliberate вернул ошибку: %v", err)
	}

	want := []trace.EventType{
		trace.ContextBuilt,
		trace.ProposalStarted,
		trace.ProposalCompleted,
		trace.CritiqueStarted,
		trace.CritiqueCompleted,
		trace.RevisionCompleted,
		trace.ConsensusEvaluated,
	}
	got := rec.types()
	if len(got) != len(want) {
		t.Fatalf("ожидалось %d событий, получено %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("событие %d = %s, ожидался %s (все: %v)", i, got[i], want[i], got)
		}
	}

	var consensusEv *trace.Event
	for i := range rec.events {
		if rec.events[i].Type == trace.ConsensusEvaluated {
			consensusEv = &rec.events[i]
		}
	}
	if consensusEv == nil {
		t.Fatal("событие ConsensusEvaluated не найдено")
	}
	if consensusEv.Metadata["agreement"] != "CONSENSUS" {
		t.Fatalf("metadata agreement = %q", consensusEv.Metadata["agreement"])
	}
	if consensusEv.TraceID != "run-1" || consensusEv.TaskID != "t-1" {
		t.Fatalf("идентификаторы события неверны: %+v", consensusEv)
	}
}

// TestEngineTraceWithoutRecorder — без рекордера движок работает.
func TestEngineTraceWithoutRecorder(t *testing.T) {
	mockA := llm.NewMock()
	mockB := llm.NewMock()
	engine, err := debate.NewEngine(debate.EngineConfig{
		ParticipantA: debate.NewParticipant("a", mockA),
		ParticipantB: debate.NewParticipant("b", mockB),
		ContextBuilder: ctxb.New(),
		ConsensusThreshold: 0.8,
		MaxRounds: map[domain.TaskMode]int{
			domain.NORMAL: 1, domain.DELIBERATE: 3, domain.CRITICAL: 6,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	scriptConsensusRound(mockA, mockB,
		verdictJSON("CONSENSUS", "SQLite", 0.9),
		verdictJSON("CONSENSUS", "SQLite", 0.9))

	task, err := domain.NewTask("t-1", "Заголовок", "", nil, domain.NORMAL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Deliberate(context.Background(), task, "run-1"); err != nil {
		t.Fatalf("Deliberate вернул ошибку: %v", err)
	}
}