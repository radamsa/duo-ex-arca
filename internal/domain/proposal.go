package domain

import (
	"fmt"
	"math"
)

// Proposal — структурированное решение участника дебата.
// Структура задана в docs/design.md, раздел 11.
type Proposal struct {
	ID            string `json:"id"`
	ParticipantID string `json:"participant_id"`

	Decision    string `json:"decision"`
	Arguments   []string `json:"arguments"`
	Assumptions []string `json:"assumptions"`
	Risks       []string `json:"risks"`

	Confidence float64 `json:"confidence"`
}

// NewProposal создаёт предложение и проверяет обязательные поля.
func NewProposal(id, participantID, decision string, arguments, assumptions, risks []string, confidence float64) (Proposal, error) {
	if id == "" {
		return Proposal{}, fmt.Errorf("domain: пустой ID предложения")
	}
	if participantID == "" {
		return Proposal{}, fmt.Errorf("domain: пустой ID участника")
	}
	if decision == "" {
		return Proposal{}, fmt.Errorf("domain: пустое решение предложения")
	}
	if math.IsNaN(confidence) || confidence < 0 || confidence > 1 {
		return Proposal{}, fmt.Errorf("domain: уверенность %v вне диапазона [0,1]", confidence)
	}
	return Proposal{
		ID:            id,
		ParticipantID: participantID,
		Decision:      decision,
		Arguments:     arguments,
		Assumptions:   assumptions,
		Risks:         risks,
		Confidence:    confidence,
	}, nil
}