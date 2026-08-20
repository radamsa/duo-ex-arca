// Тесты Consensus Engine: объединение вердиктов в итоговое решение.
package debate_test

import (
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/debate"
	"github.com/radamsa/duo-ex-arca/internal/domain"
)

func newConsensusEngine(t *testing.T, threshold float64) *debate.ConsensusEngine {
	t.Helper()
	e, err := debate.NewConsensusEngine(threshold)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// TestEvaluateConsensus — оба вердикта CONSENSUS с одинаковым решением.
func TestEvaluateConsensus(t *testing.T) {
	e := newConsensusEngine(t, 0.8)

	v1 := debate.ConsensusVerdict{
		Agreement: debate.AgreementConsensus,
		Decision:  "SQLite с WAL",
		Requirements: []string{"без C toolchain"},
		Arguments:    []string{"низкая стоимость"},
		Risks:        []string{"один файл"},
		Confidence:   0.9,
	}
	v2 := debate.ConsensusVerdict{
		Agreement: debate.AgreementConsensus,
		Decision:  "SQLite с WAL",
		Requirements: []string{"без C toolchain"},
		Arguments:    []string{"надёжность"},
		Risks:        []string{"один файл"},
		Confidence:   0.95,
	}

	decision, err := e.Evaluate(v1, v2)
	if err != nil {
		t.Fatalf("Evaluate вернул ошибку: %v", err)
	}
	if decision.Status != domain.Consensus {
		t.Fatalf("Status = %s, ожидался CONSENSUS", decision.Status)
	}
	if decision.Decision != "SQLite с WAL" {
		t.Fatalf("Decision = %q", decision.Decision)
	}
	// Уверенность консенсуса — минимальная из двух (консервативно).
	if decision.Confidence != 0.9 {
		t.Fatalf("Confidence = %v, ожидалась 0.9", decision.Confidence)
	}
	// Аргументы и риски объединяются.
	if len(decision.SupportingArguments) != 2 {
		t.Fatalf("SupportingArguments = %+v", decision.SupportingArguments)
	}
	if len(decision.Risks) != 1 {
		t.Fatalf("Risks = %+v", decision.Risks)
	}
}

// TestEvaluateConsensusBelowThreshold — уверенность ниже порога — не консенсус.
func TestEvaluateConsensusBelowThreshold(t *testing.T) {
	e := newConsensusEngine(t, 0.8)

	verdict := func(conf float64) debate.ConsensusVerdict {
		return debate.ConsensusVerdict{
			Agreement:  debate.AgreementConsensus,
			Decision:   "SQLite",
			Confidence: conf,
		}
	}

	decision, err := e.Evaluate(verdict(0.7), verdict(0.9))
	if err != nil {
		t.Fatalf("Evaluate вернул ошибку: %v", err)
	}
	if decision.Status != domain.Disagreement {
		t.Fatalf("Status = %s, ожидался DISAGREEMENT", decision.Status)
	}
}

// TestEvaluateConsensusDifferentTexts — одинаковый статус, разные тексты
// решений — ложный консенсус запрещён (TASK-062).
func TestEvaluateConsensusDifferentTexts(t *testing.T) {
	e := newConsensusEngine(t, 0.8)

	v1 := debate.ConsensusVerdict{Agreement: debate.AgreementConsensus, Decision: "SQLite", Confidence: 0.9}
	v2 := debate.ConsensusVerdict{Agreement: debate.AgreementConsensus, Decision: "PostgreSQL", Confidence: 0.9}

	decision, err := e.Evaluate(v1, v2)
	if err != nil {
		t.Fatalf("Evaluate вернул ошибку: %v", err)
	}
	if decision.Status != domain.Disagreement {
		t.Fatalf("Status = %s, ожидался DISAGREEMENT", decision.Status)
	}
}

// TestEvaluateMixedVerdicts — один консенсус, другой нет — разногласие.
func TestEvaluateMixedVerdicts(t *testing.T) {
	e := newConsensusEngine(t, 0.8)

	v1 := debate.ConsensusVerdict{Agreement: debate.AgreementConsensus, Decision: "SQLite", Confidence: 0.9}
	v2 := debate.ConsensusVerdict{Agreement: debate.AgreementDisagreement, Confidence: 0.5}

	decision, err := e.Evaluate(v1, v2)
	if err != nil {
		t.Fatalf("Evaluate вернул ошибку: %v", err)
	}
	if decision.Status != domain.Disagreement {
		t.Fatalf("Status = %s, ожидался DISAGREEMENT", decision.Status)
	}
}

// TestEvaluateInsufficientData — обеим сторонам не хватает данных.
func TestEvaluateInsufficientData(t *testing.T) {
	e := newConsensusEngine(t, 0.8)

	v1 := debate.ConsensusVerdict{Agreement: debate.AgreementInsufficientData, Confidence: 0.4}
	v2 := debate.ConsensusVerdict{Agreement: debate.AgreementInsufficientData, Confidence: 0.4}

	decision, err := e.Evaluate(v1, v2)
	if err != nil {
		t.Fatalf("Evaluate вернул ошибку: %v", err)
	}
	if decision.Status != domain.InsufficientData {
		t.Fatalf("Status = %s, ожидался INSUFFICIENT_DATA", decision.Status)
	}
}

// TestEvaluatePartialInsufficientData — одна сторона «нет данных»,
// другая «разногласие» — общий итог «разногласие».
func TestEvaluatePartialInsufficientData(t *testing.T) {
	e := newConsensusEngine(t, 0.8)

	v1 := debate.ConsensusVerdict{Agreement: debate.AgreementInsufficientData, Confidence: 0.4}
	v2 := debate.ConsensusVerdict{Agreement: debate.AgreementDisagreement, Confidence: 0.5}

	decision, err := e.Evaluate(v1, v2)
	if err != nil {
		t.Fatalf("Evaluate вернул ошибку: %v", err)
	}
	if decision.Status != domain.Disagreement {
		t.Fatalf("Status = %s, ожидался DISAGREEMENT", decision.Status)
	}
}

// TestEvaluateConsensusRequiresDecisionText — консенсус без текста решения
// невозможен.
func TestEvaluateConsensusRequiresDecisionText(t *testing.T) {
	e := newConsensusEngine(t, 0.8)

	v1 := debate.ConsensusVerdict{Agreement: debate.AgreementConsensus, Decision: "", Confidence: 0.9}
	v2 := debate.ConsensusVerdict{Agreement: debate.AgreementConsensus, Decision: "", Confidence: 0.9}

	if _, err := e.Evaluate(v1, v2); err == nil {
		t.Fatal("ожидалась ошибка: консенсус без текста решения")
	}
}

// TestNewConsensusEngineInvalidThreshold — порог вне диапазона — ошибка.
func TestNewConsensusEngineInvalidThreshold(t *testing.T) {
	for _, threshold := range []float64{-0.1, 0, 1.1} {
		if _, err := debate.NewConsensusEngine(threshold); err == nil {
			t.Errorf("порог %v должен быть невалидным", threshold)
		}
	}
	for _, threshold := range []float64{0.1, 0.5, 1} {
		if _, err := debate.NewConsensusEngine(threshold); err != nil {
			t.Errorf("порог %v должен быть валидным: %v", threshold, err)
		}
	}
}