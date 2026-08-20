// Детерминированные эвалуаторы (docs/plan-mvp.md, Этап 17).
//
// Приоритет — deterministic evaluation перед LLM judge:
// для MVP достаточно точного сравнения нормализованного текста решения
// с ожиданием из датасета. Задачи без ожидания не оцениваются (-1).
package bench

import (
	"strings"

	"github.com/radamsa/duo-ex-arca/internal/domain"
)

// scoreUnknown — задача не имеет ожидания, оценка исключается из accuracy.
const scoreUnknown = -1.0

// Evaluator — детерминированный эвалуатор.
type Evaluator struct{}

// NewEvaluator создаёт эвалуатор.
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// Score возвращает 1.0 при совпадении решения с ожиданием, 0.0 иначе.
// Пустое ожидание (в датасете нет expected) даёт -1.0 — задача не оценивается.
// Решения со статусами, отличными от CONSENSUS, считаются неверными.
func (e *Evaluator) Score(expected string, decision domain.Decision) float64 {
	if strings.TrimSpace(expected) == "" {
		return scoreUnknown
	}
	if decision.Status != domain.Consensus {
		return 0
	}
	if normalize(expected) == normalize(decision.Decision) {
		return 1
	}
	return 0
}

// normalize приводит текст решения к сопоставимому виду:
// нижний регистр, схлопнутые пробелы.
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}