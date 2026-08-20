// Инструментирование LLM-вызовов для подсчёта токенов (TASK-153).
//
// Метрики собираются наблюдателем поверх интерфейса llm.LLM, не трогая
// Debate Core: участники остаются чистыми, а обёртка добавляется
// только в wiring бенчмарка.
package bench

import (
	"context"
	"sync"

	"github.com/radamsa/duo-ex-arca/internal/llm"
)

// TokenCounter — потокобезопасный накопитель токенов.
// Дебат вызывает обоих участников параллельно (TASK-051),
// поэтому агрегация защищена мьютексом.
type TokenCounter struct {
	mu    sync.Mutex
	total int
}

// NewTokenCounter создаёт счётчик.
func NewTokenCounter() *TokenCounter {
	return &TokenCounter{}
}

// Reset обнуляет накопленные токены (обычно перед новой задачей).
func (c *TokenCounter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total = 0
}

// add накапливает токены одного ответа.
func (c *TokenCounter) add(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total += n
}

// Total возвращает суммарное количество токенов.
func (c *TokenCounter) Total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

// measuringLLM — обёртка над llm.LLM, учитывающая usage ответов.
type measuringLLM struct {
	llm     llm.LLM
	counter *TokenCounter
}

// Generate передаёт запрос нижележащему LLM и накапливает токены.
// Usage не учитывается при ошибке вызова.
func (m *measuringLLM) Generate(ctx context.Context, request llm.GenerationRequest) (llm.GenerationResponse, error) {
	response, err := m.llm.Generate(ctx, request)
	if err == nil {
		m.counter.add(response.Usage.PromptTokens + response.Usage.CompletionTokens)
	}
	return response, err
}

// Instrument оборачивает LLM для подсчёта токенов в counter.
func Instrument(model llm.LLM, counter *TokenCounter) llm.LLM {
	return &measuringLLM{llm: model, counter: counter}
}