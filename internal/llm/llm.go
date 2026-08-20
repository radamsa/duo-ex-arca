// Пакет llm — абстракция LLM-провайдера.
//
// Внутренний код зависит только от интерфейса LLM,
// а не от названий конкретных API (docs/design.md, раздел 5).
package llm

import "context"

// LLM — единый контракт генерации ответа.
type LLM interface {
	Generate(ctx context.Context, request GenerationRequest) (GenerationResponse, error)
}