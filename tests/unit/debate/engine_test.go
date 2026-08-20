// Тесты Debate Engine: полный протокол дебата с Mock LLM.
package debate_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ctxb "github.com/radamsa/duo-ex-arca/internal/context"
	"github.com/radamsa/duo-ex-arca/internal/debate"
	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/llm"
)

// verdictJSON собирает JSON-вердикт из параметров.
func verdictJSON(agreement string, decision string, conf float64) string {
	v := debate.ConsensusVerdict{
		Agreement:  debate.Agreement(agreement),
		Decision:   decision,
		Confidence: conf,
		Arguments:  []string{"общий аргумент"},
		Risks:      []string{"общий риск"},
	}
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// allMessages объединяет содержимое сообщений указанного вызова.
func allMessages(m *llm.Mock, idx int) string {
	calls := m.Calls()
	if idx >= len(calls) {
		return ""
	}
	var sb strings.Builder
	for _, msg := range calls[idx].Messages {
		sb.WriteString(msg.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}

func errorsNew(msg string) error {
	return errors.New(msg)
}

// scriptConsensusRound добавляет полный сценарий одного раунда:
// proposals, critiques, revisions, verdicts.
func scriptConsensusRound(mockA, mockB *llm.Mock, verdictA, verdictB string) {
	pa := domain.Proposal{Decision: "Использовать SQLite", Arguments: []string{"простота"}, Confidence: 0.9}
	pb := domain.Proposal{Decision: "Использовать SQLite", Arguments: []string{"простота"}, Confidence: 0.9}
	mockA.Respond(debate.ProposalToJSON(pa), llm.Usage{})
	mockB.Respond(debate.ProposalToJSON(pb), llm.Usage{})

	mockA.Respond(debate.CritiqueToJSON(domain.Critique{Errors: []string{"нет оценки нагрузки"}}), llm.Usage{})
	mockB.Respond(debate.CritiqueToJSON(domain.Critique{Errors: []string{"нет оценки нагрузки"}}), llm.Usage{})

	ra := domain.Proposal{Decision: "SQLite с WAL", Arguments: []string{"надёжность"}, Confidence: 0.9}
	rb := domain.Proposal{Decision: "SQLite с WAL", Arguments: []string{"надёжность"}, Confidence: 0.9}
	mockA.Respond(debate.ProposalToJSON(ra), llm.Usage{})
	mockB.Respond(debate.ProposalToJSON(rb), llm.Usage{})

	mockA.Respond(verdictA, llm.Usage{})
	mockB.Respond(verdictB, llm.Usage{})
}

// newTestEngine собирает движок на двух независимых моках:
// каждый участник получает только свои вызовы.
func newTestEngine(t *testing.T, maxRounds int, mode domain.TaskMode) (*debate.Engine, *llm.Mock, *llm.Mock, domain.Task) {
	t.Helper()

	mockA := llm.NewMock()
	mockB := llm.NewMock()

	engine, err := debate.NewEngine(debate.EngineConfig{
		ParticipantA: debate.NewParticipant("participant-a", mockA),
		ParticipantB: debate.NewParticipant("participant-b", mockB),
		ContextBuilder: ctxb.New(),
		ConsensusThreshold: 0.8,
		MaxRounds: map[domain.TaskMode]int{
			domain.NORMAL:   maxRounds,
			domain.DELIBERATE: maxRounds,
			domain.CRITICAL: maxRounds,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	task, err := domain.NewTask("t-1", "Какую БД выбрать?", "Нужна СУБД для Go-проекта", []string{"только Go"}, mode)
	if err != nil {
		t.Fatal(err)
	}
	return engine, mockA, mockB, task
}

// TestDeliberateConsensusFirstRound — консенсус с первого раунда.
func TestDeliberateConsensusFirstRound(t *testing.T) {
	engine, mockA, mockB, task := newTestEngine(t, 3, domain.NORMAL)
	scriptConsensusRound(mockA, mockB,
		verdictJSON("CONSENSUS", "SQLite с WAL", 0.9),
		verdictJSON("CONSENSUS", "SQLite с WAL", 0.9))

	decided, err := engine.Deliberate(context.Background(), task, "run-test")
	if err != nil {
		t.Fatalf("Deliberate вернул ошибку: %v", err)
	}

	if decided.RoundsCount() != 1 {
		t.Fatalf("ожидался 1 раунд, получено %d", decided.RoundsCount())
	}
	if !decided.HasDecision() || decided.Decision.Status != domain.Consensus {
		t.Fatalf("решение не консенсус: %+v", decided.Decision)
	}
	if decided.Decision.Decision != "SQLite с WAL" {
		t.Fatalf("текст решения = %q", decided.Decision.Decision)
	}
	if len(mockA.Calls()) != 4 || len(mockB.Calls()) != 4 {
		t.Fatalf("ожидалось по 4 вызова на участника (A: %d, B: %d)",
			len(mockA.Calls()), len(mockB.Calls()))
	}
}

// TestDeliberateRoundStructure — все фазы раунда заполнены и связаны.
func TestDeliberateRoundStructure(t *testing.T) {
	engine, mockA, mockB, task := newTestEngine(t, 1, domain.NORMAL)
	scriptConsensusRound(mockA, mockB,
		verdictJSON("CONSENSUS", "SQLite с WAL", 0.9),
		verdictJSON("CONSENSUS", "SQLite с WAL", 0.9))

	decided, err := engine.Deliberate(context.Background(), task, "run-test")
	if err != nil {
		t.Fatalf("Deliberate вернул ошибку: %v", err)
	}

	round := decided.Rounds[0]
	if !round.IsComplete() {
		t.Fatal("раунд должен быть полным")
	}
	if round.ProposalA.Decision != "Использовать SQLite" {
		t.Fatalf("ProposalA = %+v", round.ProposalA)
	}
	if round.ProposalA.ParticipantID != "participant-a" || round.ProposalB.ParticipantID != "participant-b" {
		t.Fatalf("участники привязаны неверно: %s / %s",
			round.ProposalA.ParticipantID, round.ProposalB.ParticipantID)
	}
	if round.RevisionA.Decision != "SQLite с WAL" {
		t.Fatalf("RevisionA = %+v", round.RevisionA)
	}
}

// TestDeliberateDisagreementThenConsensus — второй раунд приносит консенсус.
func TestDeliberateDisagreementThenConsensus(t *testing.T) {
	engine, mockA, mockB, task := newTestEngine(t, 3, domain.NORMAL)
	scriptConsensusRound(mockA, mockB,
		verdictJSON("DISAGREEMENT", "", 0.4),
		verdictJSON("DISAGREEMENT", "", 0.4))
	scriptConsensusRound(mockA, mockB,
		verdictJSON("CONSENSUS", "SQLite", 0.9),
		verdictJSON("CONSENSUS", "SQLite", 0.9))

	decided, err := engine.Deliberate(context.Background(), task, "run-test")
	if err != nil {
		t.Fatalf("Deliberate вернул ошибку: %v", err)
	}

	if decided.RoundsCount() != 2 {
		t.Fatalf("ожидалось 2 раунда, получено %d", decided.RoundsCount())
	}
	if decided.Decision.Status != domain.Consensus {
		t.Fatalf("решение не консенсус: %+v", decided.Decision)
	}
}

// TestDeliberateDisagreementLimit — лимит раундов исчерпан — разногласие.
func TestDeliberateDisagreementLimit(t *testing.T) {
	engine, mockA, mockB, task := newTestEngine(t, 2, domain.NORMAL)
	scriptConsensusRound(mockA, mockB,
		verdictJSON("DISAGREEMENT", "", 0.5),
		verdictJSON("DISAGREEMENT", "", 0.5))
	scriptConsensusRound(mockA, mockB,
		verdictJSON("DISAGREEMENT", "", 0.5),
		verdictJSON("DISAGREEMENT", "", 0.5))

	decided, err := engine.Deliberate(context.Background(), task, "run-test")
	if err != nil {
		t.Fatalf("Deliberate вернул ошибку: %v", err)
	}

	if decided.RoundsCount() != 2 {
		t.Fatalf("ожидалось 2 раунда, получено %d", decided.RoundsCount())
	}
	if decided.Decision.Status != domain.Disagreement {
		t.Fatalf("Status = %s, ожидался DISAGREEMENT", decided.Decision.Status)
	}
}

// TestDeliberateInsufficientData — обе стороны без данных — ранний выход.
func TestDeliberateInsufficientData(t *testing.T) {
	engine, mockA, mockB, task := newTestEngine(t, 3, domain.NORMAL)
	scriptConsensusRound(mockA, mockB,
		verdictJSON("INSUFFICIENT_DATA", "", 0.3),
		verdictJSON("INSUFFICIENT_DATA", "", 0.3))

	decided, err := engine.Deliberate(context.Background(), task, "run-test")
	if err != nil {
		t.Fatalf("Deliberate вернул ошибку: %v", err)
	}

	if decided.RoundsCount() != 1 {
		t.Fatalf("ожидался 1 раунд, получено %d", decided.RoundsCount())
	}
	if decided.Decision.Status != domain.InsufficientData {
		t.Fatalf("Status = %s, ожидался INSUFFICIENT_DATA", decided.Decision.Status)
	}
}

// TestDeliberateEarlyTermination — при консенсусе лишних вызовов нет (TASK-056).
func TestDeliberateEarlyTermination(t *testing.T) {
	engine, mockA, mockB, task := newTestEngine(t, 3, domain.NORMAL)
	scriptConsensusRound(mockA, mockB,
		verdictJSON("CONSENSUS", "SQLite", 0.9),
		verdictJSON("CONSENSUS", "SQLite", 0.9))
	scriptConsensusRound(mockA, mockB,
		verdictJSON("CONSENSUS", "SQLite", 0.9),
		verdictJSON("CONSENSUS", "SQLite", 0.9))

	decided, err := engine.Deliberate(context.Background(), task, "run-test")
	if err != nil {
		t.Fatalf("Deliberate вернул ошибку: %v", err)
	}
	if decided.RoundsCount() != 1 {
		t.Fatalf("движок должен остановиться после консенсуса, раундов: %d", decided.RoundsCount())
	}
}

// TestInitialProposalsIndependent — обязательный тест TASK-041:
// контекст initial proposal A не содержит ответа B и наоборот.
func TestInitialProposalsIndependent(t *testing.T) {
	engine, mockA, mockB, task := newTestEngine(t, 1, domain.NORMAL)

	// Намеренно разные предложения: если ответы смешаются, тест это увидит.
	pa := domain.Proposal{Decision: "СЕКРЕТ РЕШЕНИЯ A", Confidence: 0.9}
	pb := domain.Proposal{Decision: "СЕКРЕТ РЕШЕНИЯ B", Confidence: 0.9}
	mockA.Respond(debate.ProposalToJSON(pa), llm.Usage{})
	mockB.Respond(debate.ProposalToJSON(pb), llm.Usage{})
	mockA.Respond(debate.CritiqueToJSON(domain.Critique{Errors: []string{"x"}}), llm.Usage{})
	mockB.Respond(debate.CritiqueToJSON(domain.Critique{Errors: []string{"x"}}), llm.Usage{})
	mockA.Respond(debate.ProposalToJSON(pa), llm.Usage{})
	mockB.Respond(debate.ProposalToJSON(pb), llm.Usage{})
	mockA.Respond(verdictJSON("CONSENSUS", "SQLite", 0.9), llm.Usage{})
	mockB.Respond(verdictJSON("CONSENSUS", "SQLite", 0.9), llm.Usage{})

	if _, err := engine.Deliberate(context.Background(), task, "run-test"); err != nil {
		t.Fatalf("Deliberate вернул ошибку: %v", err)
	}

	callsA := mockA.Calls()
	callsB := mockB.Calls()

	proposalPromptA := allMessages(mockA, 0)
	proposalPromptB := allMessages(mockB, 0)

	if strings.Contains(proposalPromptA, "СЕКРЕТ РЕШЕНИЯ B") {
		t.Fatal("контекст предложения A содержит ответ B — изоляция нарушена")
	}
	if strings.Contains(proposalPromptB, "СЕКРЕТ РЕШЕНИЯ A") {
		t.Fatal("контекст предложения B содержит ответ A — изоляция нарушена")
	}
	if len(callsA) != 4 || len(callsB) != 4 {
		t.Fatalf("ожидалось по 4 вызова на участника (A: %d, B: %d)", len(callsA), len(callsB))
	}
}

// TestDeliberateProposalFailure — отказ участника на фазе proposal.
func TestDeliberateProposalFailure(t *testing.T) {
	engine, mockA, mockB, task := newTestEngine(t, 1, domain.NORMAL)

	// Б мокается на первом вызове.
	mockA.Respond("любой ответ", llm.Usage{})
	mockB.Fail(errorsNew("модель B недоступна"))

	if _, err := engine.Deliberate(context.Background(), task, "run-test"); err == nil {
		t.Fatal("ожидалась ошибка движка при отказе участника")
	}
}

// TestDeliberateInvalidJSONResponse — невалидный JSON от участника.
func TestDeliberateInvalidJSONResponse(t *testing.T) {
	engine, mockA, mockB, task := newTestEngine(t, 1, domain.NORMAL)

	mockA.Respond("это не JSON", llm.Usage{})
	mockB.Respond("это не JSON", llm.Usage{})

	if _, err := engine.Deliberate(context.Background(), task, "run-test"); err == nil {
		t.Fatal("ожидалась ошибка при невалидном JSON")
	}
}

// TestDeliberateFastUnsupported — движок не обслуживает режим FAST.
func TestDeliberateFastUnsupported(t *testing.T) {
	engine, mockA, mockB, task := newTestEngine(t, 1, domain.FAST)
	if _, err := engine.Deliberate(context.Background(), task, "run-test"); err == nil {
		t.Fatal("движок должен отвергнуть режим FAST")
	}
	if len(mockA.Calls()) != 0 || len(mockB.Calls()) != 0 {
		t.Fatalf("не должно быть вызовов LLM (A: %d, B: %d)", len(mockA.Calls()), len(mockB.Calls()))
	}
}

// TestNewEngineValidation — конфигурация движка проверяется.
func TestNewEngineValidation(t *testing.T) {
	if _, err := debate.NewEngine(debate.EngineConfig{}); err == nil {
		t.Fatal("пустая конфигурация должна быть невалидной")
	}
	if _, err := debate.NewEngine(debate.EngineConfig{
		ParticipantA:      debate.NewParticipant("a", llm.NewMock()),
		ParticipantB:      debate.NewParticipant("b", llm.NewMock()),
		ContextBuilder:    ctxb.New(),
		ConsensusThreshold: 0.8,
	}); err == nil {
		t.Fatal("отсутствие лимита раундов для режимов должно быть невалидным")
	}
}