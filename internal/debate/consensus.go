package debate

import (
	"fmt"
	"strings"

	"github.com/radamsa/duo-ex-arca/internal/domain"
)

// ConsensusEngine объединяет два вердикта участников в итоговое решение.
//
// Правила (docs/plan-mvp.md, TASK-060..063):
//   - CONSENSUS + CONSENSUS с одинаковым решением и уверенностью не ниже
//     порога -> итог CONSENSUS;
//   - CONSENSUS + CONSENSUS, но разные тексты решения -> DISAGREEMENT
//     (ложный консенсус запрещён);
//   - INSUFFICIENT_DATA + INSUFFICIENT_DATA -> INSUFFICIENT_DATA;
//   - любое другое сочетание -> DISAGREEMENT.
type ConsensusEngine struct {
	threshold float64
}

// NewConsensusEngine создаёт движок с порогом уверенности из (0,1].
func NewConsensusEngine(threshold float64) (*ConsensusEngine, error) {
	if threshold <= 0 || threshold > 1 {
		return nil, fmt.Errorf("debate: порог консенсуса %v вне диапазона (0,1]", threshold)
	}
	return &ConsensusEngine{threshold: threshold}, nil
}

// Evaluate вычисляет итоговое решение по двум вердиктам.
func (e *ConsensusEngine) Evaluate(v1, v2 ConsensusVerdict) (domain.Decision, error) {
	if v1.Agreement == AgreementConsensus && v2.Agreement == AgreementConsensus {
		return e.evaluateConsensus(v1, v2)
	}
	if v1.Agreement == AgreementInsufficientData && v2.Agreement == AgreementInsufficientData {
		return domain.NewDecision(domain.InsufficientData, "", minConfidence(v1, v2))
	}
	return domain.NewDecision(domain.Disagreement, "", minConfidence(v1, v2))
}

// evaluateConsensus — оба участника объявили консенсус.
func (e *ConsensusEngine) evaluateConsensus(v1, v2 ConsensusVerdict) (domain.Decision, error) {
	d1 := strings.TrimSpace(v1.Decision)
	d2 := strings.TrimSpace(v2.Decision)
	if d1 == "" || d2 == "" {
		return domain.Decision{}, fmt.Errorf("debate: вердикт консенсуса без текста решения")
	}

	confidence := minConfidence(v1, v2)
	if d1 != d2 {
		// Разные тексты решений при «консенсусе» — ложный консенсус,
		// искусственный выбор победителя запрещён (инвариант I6).
		return domain.NewDecision(domain.Disagreement, "", confidence)
	}
	if confidence < e.threshold {
		return domain.NewDecision(domain.Disagreement, "", confidence)
	}

	decision, err := domain.NewDecision(domain.Consensus, d1, confidence)
	if err != nil {
		return domain.Decision{}, err
	}
	decision.SupportingArguments = union(v1.Arguments, v2.Arguments)
	decision.Risks = union(v1.Risks, v2.Risks)
	return decision, nil
}

// minConfidence — консервативная уверенность итога.
func minConfidence(v1, v2 ConsensusVerdict) float64 {
	if v1.Confidence < v2.Confidence {
		return v1.Confidence
	}
	return v2.Confidence
}

// union объединяет списки, сохраняя порядок появления и без дублей.
func union(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	var result []string
	for _, item := range append(append([]string{}, a...), b...) {
		if seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}