package domain

import "fmt"

// DebateRound — один раунд дебата: независимые предложения,
// взаимная критика и пересмотренные версии.
type DebateRound struct {
	Number int `json:"number"`

	// ProposalA/ProposalB — независимые первоначальные предложения.
	ProposalA *Proposal `json:"proposal_a,omitempty"`
	ProposalB *Proposal `json:"proposal_b,omitempty"`

	// CritiqueA — критика предложения B участником A.
	// CritiqueB — критика предложения A участником B.
	CritiqueA *Critique `json:"critique_a,omitempty"`
	CritiqueB *Critique `json:"critique_b,omitempty"`

	// RevisionA/RevisionB — пересмотренные после критики версии.
	RevisionA *Proposal `json:"revision_a,omitempty"`
	RevisionB *Proposal `json:"revision_b,omitempty"`
}

// NewDebateRound создаёт раунд с номером >= 1.
func NewDebateRound(number int) (DebateRound, error) {
	if number < 1 {
		return DebateRound{}, fmt.Errorf("domain: номер раунда %d должен быть >= 1", number)
	}
	return DebateRound{Number: number}, nil
}

// IsComplete возвращает true, когда раунд прошёл все фазы протокола.
func (r DebateRound) IsComplete() bool {
	return r.ProposalA != nil &&
		r.ProposalB != nil &&
		r.CritiqueA != nil &&
		r.CritiqueB != nil &&
		r.RevisionA != nil &&
		r.RevisionB != nil
}

// Debate — вся сессия дебата по одной задаче.
type Debate struct {
	ID   string `json:"id"`
	Task Task `json:"task"`

	Rounds   []DebateRound `json:"rounds"`
	Decision *Decision `json:"decision,omitempty"`
}

// NewDebate создаёт дебат для валидной задачи.
func NewDebate(id string, task Task) (Debate, error) {
	if id == "" {
		return Debate{}, fmt.Errorf("domain: пустой ID дебата")
	}
	if task.ID == "" || task.Title == "" || !task.Mode.Valid() {
		return Debate{}, fmt.Errorf("domain: невалидная задача для дебата")
	}
	return Debate{
		ID:   id,
		Task: task,
	}, nil
}

// AddRound добавляет раунд; номера раундов не должны повторяться.
func (d *Debate) AddRound(round DebateRound) error {
	if round.Number < 1 {
		return fmt.Errorf("domain: номер раунда %d должен быть >= 1", round.Number)
	}
	for _, existing := range d.Rounds {
		if existing.Number == round.Number {
			return fmt.Errorf("domain: раунд %d уже существует", round.Number)
		}
	}
	d.Rounds = append(d.Rounds, round)
	return nil
}

// RoundsCount возвращает количество проведённых раундов.
func (d Debate) RoundsCount() int {
	return len(d.Rounds)
}

// SetDecision фиксирует итоговое решение дебата.
func (d *Debate) SetDecision(decision Decision) {
	d.Decision = &decision
}

// HasDecision возвращает true, если решение уже зафиксировано.
func (d Debate) HasDecision() bool {
	return d.Decision != nil
}