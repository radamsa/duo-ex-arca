// Тесты детерминированного Mock LLM.
package llm_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/llm"
)

// TestMockScriptedResponses — сценарий ответов выполняется по порядку.
func TestMockScriptedResponses(t *testing.T) {
	m := llm.NewMock().
		Respond("первый ответ", llm.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}).
		Respond("второй ответ", llm.Usage{})

	req := llm.GenerationRequest{
		Model:    "mock-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "вопрос"}},
	}

	first, err := m.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("первый вызов вернул ошибку: %v", err)
	}
	if first.Content != "первый ответ" {
		t.Fatalf("неожиданный первый ответ: %q", first.Content)
	}
	if first.Usage.TotalTokens != 3 {
		t.Fatalf("неожиданный usage: %+v", first.Usage)
	}

	second, err := m.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("второй вызов вернул ошибку: %v", err)
	}
	if second.Content != "второй ответ" {
		t.Fatalf("неожиданный второй ответ: %q", second.Content)
	}
}

// TestMockScriptedError — запланированная ошибка возвращается как есть.
func TestMockScriptedError(t *testing.T) {
	want := errors.New("модель перегружена")
	m := llm.NewMock().Fail(want)

	_, err := m.Generate(context.Background(), llm.GenerationRequest{})
	if err == nil {
		t.Fatal("ожидалась запланированная ошибка")
	}
	if !errors.Is(err, want) {
		t.Fatalf("ошибка не совпадает с запланированной: %v", err)
	}
}

// TestMockEmptyScript — вызов без запланированного ответа — ошибка теста.
func TestMockEmptyScript(t *testing.T) {
	m := llm.NewMock()
	_, err := m.Generate(context.Background(), llm.GenerationRequest{})
	if err == nil {
		t.Fatal("вызов с пустым сценарием должен вернуть ошибку")
	}
	if !strings.Contains(err.Error(), "сценарий") {
		t.Fatalf("ошибка должна сообщать о пустом сценарии: %v", err)
	}
}

// TestMockRecordsCalls — все запросы записываются для проверок теста.
func TestMockRecordsCalls(t *testing.T) {
	m := llm.NewMock().Respond("ответ", llm.Usage{})

	req := llm.GenerationRequest{
		Model:    "mock-model",
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: "промпт"}},
	}
	if _, err := m.Generate(context.Background(), req); err != nil {
		t.Fatalf("Generate вернул ошибку: %v", err)
	}

	calls := m.Calls()
	if len(calls) != 1 {
		t.Fatalf("ожидалась одна запись, получено %d", len(calls))
	}
	if calls[0].Model != "mock-model" {
		t.Fatalf("записанный запрос не совпадает: %+v", calls[0])
	}
	if !strings.Contains(calls[0].Messages[0].Content, "промпт") {
		t.Fatalf("содержимое запроса не записано: %+v", calls[0].Messages)
	}
}

// TestMockReset — сброс возвращает mock в исходное состояние.
func TestMockReset(t *testing.T) {
	m := llm.NewMock().Respond("ответ", llm.Usage{})
	m.Reset()

	if len(m.Calls()) != 0 {
		t.Fatal("после Reset записи вызовов должны быть пустыми")
	}
	if _, err := m.Generate(context.Background(), llm.GenerationRequest{}); err == nil {
		t.Fatal("после Reset сценарий должен быть пустым")
	}
}

// TestMockConcurrentCalls — concurrent безопасность mock-а.
func TestMockConcurrentCalls(t *testing.T) {
	m := llm.NewMock()
	for i := 0; i < 10; i++ {
		m.Respond("ответ", llm.Usage{})
	}

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			if _, err := m.Generate(context.Background(), llm.GenerationRequest{}); err != nil {
				t.Errorf("concurrent Generate вернул ошибку: %v", err)
			}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	if len(m.Calls()) != 10 {
		t.Fatalf("ожидалось 10 записей, получено %d", len(m.Calls()))
	}
}