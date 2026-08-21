// Тесты Agent Runner: pipeline и режимы работы.
package agent_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/agent"
	ctxb "github.com/radamsa/duo-ex-arca/internal/context"
	"github.com/radamsa/duo-ex-arca/internal/debate"
	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/llm"
	"github.com/radamsa/duo-ex-arca/internal/trace"
)

func errorsNew(msg string) error {
	return errors.New(msg)
}

// newFastRunner плюс мок для FAST-режима.
func newRunner(t *testing.T, maxRounds int) (*agent.Runner, *llm.Mock, *llm.Mock, *llm.Mock) {
	t.Helper()

	mockFast := llm.NewMock()
	mockA := llm.NewMock()
	mockB := llm.NewMock()

	engine, err := debate.NewEngine(debate.EngineConfig{
		ParticipantA:      debate.NewParticipant("participant-a", mockA),
		ParticipantB:      debate.NewParticipant("participant-b", mockB),
		ContextBuilder:    ctxb.New(),
		ConsensusThreshold: 0.8,
		MaxRounds: map[domain.TaskMode]int{
			domain.NORMAL:     maxRounds,
			domain.DELIBERATE: maxRounds,
			domain.CRITICAL:   maxRounds,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	runner, err := agent.NewRunner(agent.RunnerConfig{
		Engine:          engine,
		FastParticipant: debate.NewParticipant("participant-fast", mockFast),
		ContextBuilder:  ctxb.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner, mockFast, mockA, mockB
}

func mustTask(t *testing.T, mode domain.TaskMode) domain.Task {
	t.Helper()
	task, err := domain.NewTask("t-1", "Какую БД выбрать?", "Нужна СУБД", nil, mode)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func verdictJSON(agreement string, decision string, conf float64) string {
	return debate.VerdictToText(debate.ConsensusVerdict{
		Agreement:  debate.Agreement(agreement),
		Decision:   decision,
		Confidence: conf,
	})
}

// scriptConsensusRound — сценарий одного консенсусного раунда.
func scriptConsensusRound(mockA, mockB *llm.Mock) {
	pa := domain.Proposal{Decision: "SQLite", Confidence: 0.9}
	pb := domain.Proposal{Decision: "SQLite", Confidence: 0.9}
	mockA.Respond(debate.ProposalToText(pa), llm.Usage{})
	mockB.Respond(debate.ProposalToText(pb), llm.Usage{})
	mockA.Respond(debate.CritiqueToText(domain.Critique{Errors: []string{"x"}}), llm.Usage{})
	mockB.Respond(debate.CritiqueToText(domain.Critique{Errors: []string{"x"}}), llm.Usage{})
	ra := domain.Proposal{Decision: "SQLite с WAL", Confidence: 0.9}
	rb := domain.Proposal{Decision: "SQLite с WAL", Confidence: 0.9}
	mockA.Respond(debate.ProposalToText(ra), llm.Usage{})
	mockB.Respond(debate.ProposalToText(rb), llm.Usage{})
	mockA.Respond(verdictJSON("CONSENSUS", "SQLite с WAL", 0.9), llm.Usage{})
	mockB.Respond(verdictJSON("CONSENSUS", "SQLite с WAL", 0.9), llm.Usage{})
}

// TestRunFast — FAST обходит Debate Core, один вызов LLM (TASK-071).
func TestRunFast(t *testing.T) {
	runner, mockFast, mockA, mockB := newRunner(t, 3)

	fastProposal := domain.Proposal{Decision: "Быстрое решение", Arguments: []string{"скорость"}, Confidence: 0.7}
	mockFast.Respond(debate.ProposalToText(fastProposal), llm.Usage{})

	decision, _, err := runner.Run(context.Background(), mustTask(t, domain.FAST))
	if err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	if decision.Status != domain.Consensus {
		t.Fatalf("Status = %s", decision.Status)
	}
	if decision.Decision != "Быстрое решение" {
		t.Fatalf("Decision = %q", decision.Decision)
	}
	if decision.Confidence != 0.7 {
		t.Fatalf("Confidence = %v", decision.Confidence)
	}
	if len(decision.SupportingArguments) != 1 {
		t.Fatalf("аргументы не проброшены: %+v", decision.SupportingArguments)
	}
	// Debate Core не вызывается вовсе.
	if len(mockA.Calls()) != 0 || len(mockB.Calls()) != 0 {
		t.Fatalf("движок дебата не должен вызываться в FAST (A: %d, B: %d)", len(mockA.Calls()), len(mockB.Calls()))
	}
	if len(mockFast.Calls()) != 1 {
		t.Fatalf("ожидался 1 вызов в FAST, получено %d", len(mockFast.Calls()))
	}
}

// TestRunNormal — NORMAL идёт через движок.
func TestRunNormal(t *testing.T) {
	runner, _, mockA, mockB := newRunner(t, 3)
	scriptConsensusRound(mockA, mockB)

	decision, _, err := runner.Run(context.Background(), mustTask(t, domain.NORMAL))
	if err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}
	if decision.Status != domain.Consensus {
		t.Fatalf("Status = %s", decision.Status)
	}
	if decision.Decision != "SQLite с WAL" {
		t.Fatalf("Decision = %q", decision.Decision)
	}
}

// TestRunDeliberate — DELIBERATE с несколькими раундами.
func TestRunDeliberate(t *testing.T) {
	runner, _, mockA, mockB := newRunner(t, 3)

	scriptConsensusRound(mockA, mockB)
	scriptConsensusRound(mockA, mockB)

	decision, _, err := runner.Run(context.Background(), mustTask(t, domain.DELIBERATE))
	if err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}
	if decision.Status != domain.Consensus {
		t.Fatalf("Status = %s", decision.Status)
	}
}

// TestRunEngineFailure — ошибка движка даёт FAILED + ошибку.
func TestRunEngineFailure(t *testing.T) {
	runner, _, mockA, mockB := newRunner(t, 1)

	mockA.Respond(debate.ProposalToText(domain.Proposal{Decision: "x", Confidence: 0.5}), llm.Usage{})
	mockB.Fail(errorsNew("модель недоступна"))

	decision, _, err := runner.Run(context.Background(), mustTask(t, domain.NORMAL))
	if err == nil {
		t.Fatal("ожидалась ошибка при отказе участника")
	}
	if decision.Status != domain.Failed {
		t.Fatalf("Status = %s, ожидался FAILED", decision.Status)
	}
}

// TestRunFastInvalidResponse — пустой ответ модели в FAST даёт FAILED.
func TestRunFastInvalidResponse(t *testing.T) {
	runner, mockFast, _, _ := newRunner(t, 3)
	mockFast.Respond("", llm.Usage{})

	decision, _, err := runner.Run(context.Background(), mustTask(t, domain.FAST))
	if err == nil {
		t.Fatal("ожидалась ошибка при пустом ответе модели")
	}
	if decision.Status != domain.Failed {
		t.Fatalf("Status = %s, ожидался FAILED", decision.Status)
	}
}

// TestRunFastLLMFailure — отказ LLM в FAST даёт FAILED.
func TestRunFastLLMFailure(t *testing.T) {
	runner, mockFast, _, _ := newRunner(t, 3)
	mockFast.Fail(errorsNew("таймаут"))

	decision, _, err := runner.Run(context.Background(), mustTask(t, domain.FAST))
	if err == nil {
		t.Fatal("ожидалась ошибка при отказе LLM")
	}
	if decision.Status != domain.Failed {
		t.Fatalf("Status = %s, ожидался FAILED", decision.Status)
	}
}

// TestRunValidation — неполная конфигурация runner'а отклоняется.
func TestRunValidation(t *testing.T) {
	if _, err := agent.NewRunner(agent.RunnerConfig{}); err == nil {
		t.Fatal("пустая конфигурация должна быть невалидной")
	}
	if _, err := agent.NewRunner(agent.RunnerConfig{
		Engine:         &debate.Engine{},
		FastParticipant: debate.NewParticipant("f", llm.NewMock()),
	}); err == nil {
		t.Fatal("отсутствие ContextBuilder должно быть невалидным")
	}
}

// memRecorder — рекордер трассировки в памяти.
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

func (m *memRecorder) types() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var types []string
	for _, ev := range m.events {
		types = append(types, string(ev.Type))
	}
	return types
}

// TestRunEmitsTraceEvents — runner пишет события жизненного цикла.
func TestRunEmitsTraceEvents(t *testing.T) {
	mockFast := llm.NewMock()
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
	})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Engine:          engine,
		FastParticipant: debate.NewParticipant("participant-fast", mockFast),
		ContextBuilder:  ctxb.New(),
		Trace:           rec,
	})
	if err != nil {
		t.Fatal(err)
	}

	mockFast.Respond(debate.ProposalToText(domain.Proposal{Decision: "x", Confidence: 0.5}), llm.Usage{})
	if _, _, err := runner.Run(context.Background(), mustTask(t, domain.FAST)); err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	got := rec.types()
	want := []string{"TASK_CREATED", "CONTEXT_BUILT", "DECISION_CREATED"}
	if len(got) != len(want) {
		t.Fatalf("ожидалось %d событий, получено %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("событие %d = %s, ожидался %s (все: %v)", i, got[i], want[i], got)
		}
	}
}

// TestRunFastPromptContainsTask — FAST передаёт задачу в промпт.
func TestRunFastPromptContainsTask(t *testing.T) {
	runner, mockFast, _, _ := newRunner(t, 3)
	fastProposal := domain.Proposal{Decision: "x", Confidence: 0.5}
	mockFast.Respond(debate.ProposalToText(fastProposal), llm.Usage{})

	if _, _, err := runner.Run(context.Background(), mustTask(t, domain.FAST)); err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	calls := mockFast.Calls()
	if len(calls) != 1 || len(calls[0].Messages) == 0 {
		t.Fatal("промпт не передан в LLM")
	}
	var all strings.Builder
	for _, m := range calls[0].Messages {
		all.WriteString(m.Content)
	}
	if !strings.Contains(all.String(), "Какую БД выбрать?") {
		t.Fatalf("промпт не содержит задачи: %s", all.String())
	}
}

// TestRunFastNotifiesActivity — FAST сообщает активность участника.
func TestRunFastNotifiesActivity(t *testing.T) {
	mockFast := llm.NewMock()
	mockA := llm.NewMock()
	mockB := llm.NewMock()

	engine, err := debate.NewEngine(debate.EngineConfig{
		ParticipantA:      debate.NewParticipant("participant-a", mockA),
		ParticipantB:      debate.NewParticipant("participant-b", mockB),
		ContextBuilder:    ctxb.New(),
		ConsensusThreshold: 0.8,
		MaxRounds: map[domain.TaskMode]int{
			domain.NORMAL:     1,
			domain.DELIBERATE: 1,
			domain.CRITICAL:   1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var gotID, gotStage string
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Engine:          engine,
		FastParticipant: debate.NewParticipant("participant-fast", mockFast),
		ContextBuilder:  ctxb.New(),
		Notify: func(participantID, stage string) {
			gotID, gotStage = participantID, stage
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	mockFast.Respond(debate.ProposalToText(domain.Proposal{Decision: "SQLite", Confidence: 0.9}), llm.Usage{})
	if _, _, err := runner.Run(context.Background(), mustTask(t, domain.FAST)); err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	if gotID != "participant-fast" {
		t.Fatalf("participantID = %q, ожидался participant-fast", gotID)
	}
	if gotStage != debate.StagePropose {
		t.Fatalf("stage = %q, ожидалась %q", gotStage, debate.StagePropose)
	}
}