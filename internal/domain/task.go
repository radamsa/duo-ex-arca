// Пакет domain — предметная модель Duo ex Arca.
//
// Не зависит от SQLite, LLM-провайдеров и любого внешнего кода.
// Все компоненты — чистые данные плюс валидация, что делает
// модель полностью тестируемой без внешних сервисов.
package domain

import "fmt"

// TaskMode — режим работы агента.
// Описан в docs/design.md, раздел 15.
type TaskMode string

const (
	// FAST — одна LLM, без дебата.
	FAST TaskMode = "FAST"
	// NORMAL — две LLM, один основной раунд.
	NORMAL TaskMode = "NORMAL"
	// DELIBERATE — две LLM, несколько раундов.
	DELIBERATE TaskMode = "DELIBERATE"
	// CRITICAL — две LLM с усиленной проверкой.
	CRITICAL TaskMode = "CRITICAL"
)

// Valid возвращает true, если режим является одним из поддерживаемых.
func (m TaskMode) Valid() bool {
	switch m {
	case FAST, NORMAL, DELIBERATE, CRITICAL:
		return true
	default:
		return false
	}
}

// Task — задача пользователя, переданная агенту.
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Constraints []string `json:"constraints"`
	Mode        TaskMode `json:"mode"`
}

// NewTask создаёт задачу и проверяет обязательные поля.
func NewTask(id, title, description string, constraints []string, mode TaskMode) (Task, error) {
	if id == "" {
		return Task{}, fmt.Errorf("domain: пустой ID задачи")
	}
	if title == "" {
		return Task{}, fmt.Errorf("domain: пустой заголовок задачи")
	}
	if !mode.Valid() {
		return Task{}, fmt.Errorf("domain: невалидный режим задачи %q", mode)
	}
	return Task{
		ID:          id,
		Title:       title,
		Description: description,
		Constraints: constraints,
		Mode:        mode,
	}, nil
}