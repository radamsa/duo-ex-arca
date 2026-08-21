// Тесты уведомлений активности: движок сообщает стадии каждого участника.
package debate_test

import (
	"context"
	"sync"
	"testing"

	ctxb "github.com/radamsa/duo-ex-arca/internal/context"
	"github.com/radamsa/duo-ex-arca/internal/debate"
	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/llm"
)

// TestEngineNotifiesActivity — движок сообщает о всех фазах участника
// в порядке протокола: proposal -> critique -> revision -> consensus.
func TestEngineNotifiesActivity(t *testing.T) {
	mockA := llm.NewMock()
	mockB := llm.NewMock()

	var (
		mu     sync.Mutex
		stages = map[string][]string{"participant-a": {}, "participant-b": {}}
	)
	notify := func(participantID, stage string) {
		mu.Lock()
		defer mu.Unlock()
		stages[participantID] = append(stages[participantID], stage)
	}

	engine, err := debate.NewEngine(debate.EngineConfig{
		ParticipantA:       debate.NewParticipant("participant-a", mockA),
		ParticipantB:       debate.NewParticipant("participant-b", mockB),
		ContextBuilder:     ctxb.New(),
		ConsensusThreshold: 0.8,
		MaxRounds: map[domain.TaskMode]int{
			domain.NORMAL:     1,
			domain.DELIBERATE: 1,
			domain.CRITICAL:   1,
		},
		Notify: notify,
	})
	if err != nil {
		t.Fatal(err)
	}

	scriptConsensusRound(mockA, mockB,
		verdictJSON("CONSENSUS", "SQLite с WAL", 0.9),
		verdictJSON("CONSENSUS", "SQLite с WAL", 0.9))

	task, err := domain.NewTask("t-1", "Какую БД выбрать?", "Нужна СУБД", []string{"только Go"}, domain.NORMAL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Deliberate(context.Background(), task, "run-test"); err != nil {
		t.Fatalf("Deliberate вернул ошибку: %v", err)
	}

	want := []string{
		debate.StagePropose,
		debate.StageCritique,
		debate.StageRevise,
		debate.StageConsensus,
	}
	mu.Lock()
	defer mu.Unlock()
	for _, id := range []string{"participant-a", "participant-b"} {
		got := stages[id]
		if len(got) != len(want) {
			t.Fatalf("%s: стадии %v, ожидалось %v", id, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: стадия[%d] = %q, ожидалась %q (все: %v)", id, i, got[i], want[i], got)
			}
		}
	}
}

// TestEngineNilNotify — движок работает без колбэка активности.
func TestEngineNilNotify(t *testing.T) {
	engine, mockA, mockB, task := newTestEngine(t, 1, domain.NORMAL)
	scriptConsensusRound(mockA, mockB,
		verdictJSON("CONSENSUS", "SQLite с WAL", 0.9),
		verdictJSON("CONSENSUS", "SQLite с WAL", 0.9))

	if _, err := engine.Deliberate(context.Background(), task, "run-test"); err != nil {
		t.Fatalf("Deliberate вернул ошибку: %v", err)
	}
}
