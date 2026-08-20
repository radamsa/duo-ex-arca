package domain

import (
	"fmt"
	"math"
)

// DecisionStatus — итоговый статус решения агента.
// Перечень задан в docs/design.md, раздел 13.
type DecisionStatus string

const (
	// Consensus — модели согласны по решению, требованиям, аргументам и рискам.
	Consensus DecisionStatus = "CONSENSUS"
	// Disagreement — модели не пришли к согласию (штатный результат).
	Disagreement DecisionStatus = "DISAGREEMENT"
	// InsufficientData — обеим моделям не хватило данных для решения.
	InsufficientData DecisionStatus = "INSUFFICIENT_DATA"
	// RequireUserInput — требуется уточнение у пользователя.
	RequireUserInput DecisionStatus = "REQUIRE_USER_INPUT"
	// Failed — обработка завершилась ошибкой.
	Failed DecisionStatus = "FAILED"
)

// Valid возвращает true, если статус является одним из поддерживаемых.
func (s DecisionStatus) Valid() bool {
	switch s {
	case Consensus, Disagreement, InsufficientData, RequireUserInput, Failed:
		return true
	default:
		return false
	}
}

// Evidence — подтверждение решения внешним источником.
type Evidence struct {
	Source      string `json:"source"`
	Description string `json:"description"`
}

// Decision — итоговое решение агента по задаче.
// Структура задана в docs/design.md, раздел 13.
type Decision struct {
	Status DecisionStatus `json:"status"`

	Decision   string `json:"decision"`
	Confidence float64 `json:"confidence"`

	SupportingArguments []string `json:"supporting_arguments"`
	RejectedArguments   []string `json:"rejected_arguments"`

	Risks            []string `json:"risks"`
	UnresolvedIssues []string `json:"unresolved_issues"`

	Evidence []Evidence `json:"evidence"`
}

// NewDecision создаёт решение и проверяет обязательные поля.
// Решение обязательно только при статусе Consensus: Disagreement,
// InsufficientData, RequireUserInput и Failed могут не содержать текста.
func NewDecision(status DecisionStatus, decision string, confidence float64) (Decision, error) {
	if !status.Valid() {
		return Decision{}, fmt.Errorf("domain: невалидный статус решения %q", status)
	}
	if status == Consensus && decision == "" {
		return Decision{}, fmt.Errorf("domain: пустое решение при статусе %s", status)
	}
	if math.IsNaN(confidence) || confidence < 0 || confidence > 1 {
		return Decision{}, fmt.Errorf("domain: уверенность %v вне диапазона [0,1]", confidence)
	}
	return Decision{
		Status:     status,
		Decision:   decision,
		Confidence: confidence,
	}, nil
}