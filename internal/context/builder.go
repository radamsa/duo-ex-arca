// Пакет context — построение контекста для участников дебата.
//
// На MVP контекст состоит из задачи и ограничений (docs/plan-mvp.md, TASK-040).
// Критично: в контекст initial proposals никогда не попадают ответы
// других участников (инвариант I5, тест TASK-041 на уровне Debate Engine).
package context

import (
	"fmt"
	"strings"

	"github.com/radamsa/duo-ex-arca/internal/domain"
)

// Builder строит текстовый контекст для LLM.
type Builder struct{}

// New создаёт ContextBuilder.
func New() *Builder {
	return &Builder{}
}

// Build формирует текст контекста: задача, описание и ограничения.
// Контекст детерминирован: одинаковые задачи дают одинаковый текст.
func (b *Builder) Build(task domain.Task) (string, error) {
	if task.Title == "" {
		return "", fmt.Errorf("context: задача без заголовка")
	}

	var sb strings.Builder
	sb.WriteString("Контекст задачи:")
	sb.WriteString("\nЗаголовок: ")
	sb.WriteString(task.Title)
	if task.Description != "" {
		sb.WriteString("\nОписание: ")
		sb.WriteString(task.Description)
	}
	if len(task.Constraints) > 0 {
		sb.WriteString("\nОграничения: ")
		sb.WriteString(strings.Join(task.Constraints, "; "))
	}
	return sb.String(), nil
}