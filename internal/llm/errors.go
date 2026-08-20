package llm

import (
	"errors"
	"fmt"
)

// ErrorKind — категория ошибки LLM-клиента.
// Категории заданы в docs/plan-mvp.md, TASK-013.
type ErrorKind string

const (
	// KindTimeout — превышение таймаута запроса.
	KindTimeout ErrorKind = "timeout"
	// KindConnection — ошибка сетевого соединения.
	KindConnection ErrorKind = "connection"
	// KindHTTP — ошибка HTTP (4xx/5xx), не попавшая в другие категории.
	KindHTTP ErrorKind = "http"
	// KindRateLimit — превышение лимита запросов (HTTP 429).
	KindRateLimit ErrorKind = "rate_limit"
	// KindInvalidResponse — ответ валиден по JSON, но структура неверная.
	KindInvalidResponse ErrorKind = "invalid_response"
	// KindInvalidJSON — тело ответа не является валидным JSON.
	KindInvalidJSON ErrorKind = "invalid_json"
	// KindContextOverflow — запрос превышает контекст модели.
	KindContextOverflow ErrorKind = "context_overflow"
)

// Error — нормализованная ошибка LLM-клиента.
type Error struct {
	Kind ErrorKind

	// StatusCode — HTTP-статус для ошибок HTTP/rate limit, иначе 0.
	StatusCode int

	Message string

	// Cause — исходная ошибка.
	Cause error
}

// NewError создаёт нормализованную ошибку.
func NewError(kind ErrorKind, statusCode int, message string, cause error) *Error {
	return &Error{
		Kind:       kind,
		StatusCode: statusCode,
		Message:    message,
		Cause:      cause,
	}
}

// Error реализует интерфейс error.
func (e *Error) Error() string {
	msg := fmt.Sprintf("llm: %s", e.Kind)
	if e.StatusCode != 0 {
		msg += fmt.Sprintf(" (HTTP %d)", e.StatusCode)
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

// Unwrap поддерживает цепочки errors.Is/errors.As.
func (e *Error) Unwrap() error {
	return e.Cause
}

// KindOf возвращает категорию ошибки или пустую строку,
// если ошибка не является нормализованной llm.Error.
func KindOf(err error) ErrorKind {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Kind
	}
	return ""
}

// Retryable возвращает true, если ошибку безопасно повторить.
// По плану TASK-111 повторяются только временные сбои:
// потеря соединения и rate limit.
func Retryable(err error) bool {
	switch KindOf(err) {
	case KindConnection, KindRateLimit:
		return true
	default:
		return false
	}
}