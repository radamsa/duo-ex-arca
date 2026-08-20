package llm

import (
	"context"
	"sync"
)

// Mock — детерминированный LLM для unit-тестов (docs/plan-mvp.md, TASK-014).
//
// Позволяет сценарировать последовательность ответов и ошибок
// (proposal, critique, revision, consensus, failure, timeout)
// и записывает все вызовы для проверок теста.
type Mock struct {
	mu        sync.Mutex
	responses []GenerationResponse
	errs      []error
	calls     []GenerationRequest
}

// NewMock создаёт пустой mock.
func NewMock() *Mock {
	return &Mock{}
}

// Respond добавляет в сценарий очередной успешный ответ.
func (m *Mock) Respond(content string, usage Usage) *Mock {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, GenerationResponse{Content: content, Usage: usage})
	return m
}

// Fail добавляет в сценарий очередную ошибку.
func (m *Mock) Fail(err error) *Mock {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errs = append(m.errs, err)
	return m
}

// Generate выполняет следующий шаг сценария и записывает запрос.
// Если сценарий исчерпан, возвращает ошибку — это сигнализирует
// о неправильной настройке теста, а не о проблеме модели.
func (m *Mock) Generate(_ context.Context, request GenerationRequest) (GenerationResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, request)

	if len(m.responses) > 0 {
		resp := m.responses[0]
		m.responses = m.responses[1:]
		return resp, nil
	}
	if len(m.errs) > 0 {
		err := m.errs[0]
		m.errs = m.errs[1:]
		return GenerationResponse{}, err
	}
	return GenerationResponse{}, &mockError{"mock: сценарий ответов исчерпан"}
}

// Calls возвращает копию всех выполненных запросов.
func (m *Mock) Calls() []GenerationRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	calls := make([]GenerationRequest, len(m.calls))
	copy(calls, m.calls)
	return calls
}

// Reset очищает сценарий и записи вызовов.
func (m *Mock) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = nil
	m.errs = nil
	m.calls = nil
}

// mockError — внутренняя ошибка mock-а.
type mockError struct{ message string }

func (e *mockError) Error() string {
	return e.message
}

var _ LLM = (*Mock)(nil)