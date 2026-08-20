package domain

import "fmt"

// Critique — структурированная критика предложения другого участника.
// Структура задана в docs/design.md, раздел 12.
type Critique struct {
	ValidPoints        []string `json:"valid_points"`
	Errors             []string `json:"errors"`
	MissingInformation []string `json:"missing_information"`
	Risks              []string `json:"risks"`
	CounterArguments   []string `json:"counter_arguments"`
	ProposedChanges    []string `json:"proposed_changes"`
}

// HasContent возвращает true, если хотя бы одно поле критики заполнено.
func (c Critique) HasContent() bool {
	return len(c.ValidPoints) > 0 ||
		len(c.Errors) > 0 ||
		len(c.MissingInformation) > 0 ||
		len(c.Risks) > 0 ||
		len(c.CounterArguments) > 0 ||
		len(c.ProposedChanges) > 0
}

// Validate проверяет, что критика содержит хотя бы одно замечание.
// Полностью пустая критика не имеет смысла для протокола дебата.
func (c Critique) Validate() error {
	if !c.HasContent() {
		return fmt.Errorf("domain: пустая критика")
	}
	return nil
}