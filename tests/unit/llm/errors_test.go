// Тесты нормализованной модели ошибок LLM-клиента.
package llm_test

import (
	"errors"
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/llm"
)

// TestKindOf — KindOf распознаёт все категории ошибок.
func TestKindOf(t *testing.T) {
	cases := []struct {
		name string
		err  error
		kind llm.ErrorKind
	}{
		{"таймаут", llm.NewError(llm.KindTimeout, 0, "истекло время", nil), llm.KindTimeout},
		{"соединение", llm.NewError(llm.KindConnection, 0, "нет сети", nil), llm.KindConnection},
		{"http", llm.NewError(llm.KindHTTP, 500, "сервер упал", nil), llm.KindHTTP},
		{"rate limit", llm.NewError(llm.KindRateLimit, 429, "слишком много запросов", nil), llm.KindRateLimit},
		{"невалидный JSON", llm.NewError(llm.KindInvalidJSON, 0, "плохой JSON", nil), llm.KindInvalidJSON},
		{"невалидный ответ", llm.NewError(llm.KindInvalidResponse, 0, "нет choices", nil), llm.KindInvalidResponse},
		{"переполнение контекста", llm.NewError(llm.KindContextOverflow, 0, "контекст переполнен", nil), llm.KindContextOverflow},
		{"nil", nil, ""},
		{"посторонняя ошибка", errors.New("прочее"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := llm.KindOf(tc.err); got != tc.kind {
				t.Fatalf("KindOf(%v) = %q, ожидалось %q", tc.err, got, tc.kind)
			}
		})
	}
}

// TestKindOfWrapped — категория распознаётся через обёртки.
func TestKindOfWrapped(t *testing.T) {
	base := llm.NewError(llm.KindRateLimit, 429, "перебор", nil)
	wrapped := errors.Join(base, errors.New("обёртка"))
	if got := llm.KindOf(wrapped); got != llm.KindRateLimit {
		t.Fatalf("KindOf обёрнутой ошибки = %q, ожидалось rate_limit", got)
	}
}

// TestRetryable — повторять можно только retryable ошибки.
func TestRetryable(t *testing.T) {
	retryable := []error{
		llm.NewError(llm.KindConnection, 0, "", nil),
		llm.NewError(llm.KindRateLimit, 429, "", nil),
	}
	for _, err := range retryable {
		if !llm.Retryable(err) {
			t.Errorf("ошибка %v должна считаться retryable", err)
		}
	}

	notRetryable := []error{
		llm.NewError(llm.KindTimeout, 0, "", nil),
		llm.NewError(llm.KindHTTP, 500, "", nil),
		llm.NewError(llm.KindInvalidJSON, 0, "", nil),
		llm.NewError(llm.KindInvalidResponse, 0, "", nil),
		llm.NewError(llm.KindContextOverflow, 0, "", nil),
		errors.New("посторонняя ошибка"),
		nil,
	}
	for _, err := range notRetryable {
		if llm.Retryable(err) {
			t.Errorf("ошибка %v не должна считаться retryable", err)
		}
	}
}

// TestErrorPreservesFields — Error сохраняет статус и причину.
func TestErrorPreservesFields(t *testing.T) {
	cause := errors.New("внутренняя причина")
	err := llm.NewError(llm.KindHTTP, 503, "сервис недоступен", cause)

	var typed *llm.Error
	if !errors.As(err, &typed) {
		t.Fatal("ошибка не приводится к *llm.Error")
	}
	if typed.StatusCode != 503 {
		t.Fatalf("StatusCode = %d, ожидалось 503", typed.StatusCode)
	}
	if !errors.Is(err, cause) {
		t.Fatal("причина не сохранилась в цепочке ошибок")
	}
	if typed.Error() == "" {
		t.Fatal("сообщение ошибки пустое")
	}
}