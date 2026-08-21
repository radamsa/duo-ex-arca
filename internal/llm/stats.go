// Сбор статистики вызовов LLM: токены и время по каждому участнику.
// Декоратор поверх llm.LLM — Debate Core остаётся чистым.
package llm

import (
	"context"
	"sync"
	"time"
)

// ParticipantStats — статистика вызовов одного участника.
type ParticipantStats struct {
	Requests         int           `json:"requests"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	TotalDuration    time.Duration `json:"-"`
	LastDuration     time.Duration `json:"-"`

	// TotalSeconds/LastSeconds — длительности в секундах для сериализации.
	TotalSeconds float64 `json:"total_seconds"`
	LastSeconds  float64 `json:"last_seconds"`
}

// TotalTokens — суммарные токены (промпт + ответ).
func (s ParticipantStats) TotalTokens() int {
	return s.PromptTokens + s.CompletionTokens
}

// withDurations возвращает копию с заполненными секундами.
func (s ParticipantStats) withDurations() ParticipantStats {
	s.TotalSeconds = s.TotalDuration.Seconds()
	s.LastSeconds = s.LastDuration.Seconds()
	return s
}

// StatsCollector — потокобезопасный сборщик статистики по участникам:
// фазы дебата выполняются параллельно.
type StatsCollector struct {
	mu   sync.Mutex
	data map[string]ParticipantStats
}

// NewStatsCollector создаёт пустой сборщик.
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{data: make(map[string]ParticipantStats)}
}

// snapshot возвращает копию статистики участника (нулевую, если нет).
func (c *StatsCollector) snapshot(participantID string) ParticipantStats {
	return c.data[participantID]
}

// Snapshot возвращает статистику участника (для чтения извне).
func (c *StatsCollector) Snapshot(participantID string) ParticipantStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshot(participantID).withDurations()
}

// All возвращает снимок статистики всех участников.
func (c *StatsCollector) All() map[string]ParticipantStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]ParticipantStats, len(c.data))
	for id, s := range c.data {
		out[id] = s.withDurations()
	}
	return out
}

// record учитывает один вызов участника. Токены учитываются только
// при успешном ответе; время — всегда (неудачный запрос тоже тратится).
func (c *StatsCollector) record(participantID string, elapsed time.Duration, usage Usage, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.snapshot(participantID)
	s.Requests++
	s.TotalDuration += elapsed
	s.LastDuration = elapsed
	if ok {
		s.PromptTokens += usage.PromptTokens
		s.CompletionTokens += usage.CompletionTokens
	}
	c.data[participantID] = s
}

// statsLLM — обёртка над LLM, пишущая статистику в collector.
type statsLLM struct {
	label    string
	inner    LLM
	collector *StatsCollector
}

// Generate измеряет длительность вызова и накапливает статистику.
func (m *statsLLM) Generate(ctx context.Context, request GenerationRequest) (GenerationResponse, error) {
	start := time.Now()
	resp, err := m.inner.Generate(ctx, request)
	m.collector.record(m.label, time.Since(start), resp.Usage, err == nil)
	return resp, err
}

// WithStats оборачивает LLM так, что каждый вызов обновляет статистику
// участника label в collector.
func WithStats(label string, inner LLM, collector *StatsCollector) LLM {
	if collector == nil {
		return inner
	}
	return &statsLLM{label: label, inner: inner, collector: collector}
}
