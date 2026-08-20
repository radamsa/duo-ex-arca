package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client — OpenAI-compatible HTTP клиент (docs/plan-mvp.md, TASK-012).
//
// Работает с любым провайдером, реализующим POST /chat/completions:
// OpenRouter, Ollama, LM Studio, llama.cpp и облачные сервисы.
// Отличается только baseURL, model и API key.
type Client struct {
	baseURL string
	model   string
	apiKey  string

	http    *http.Client
	retries int
}

// Option — настройка клиента.
type Option func(*Client)

// WithHTTPClient переопределяет HTTP-клиент (например, для тестов).
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.http = httpClient
		}
	}
}

// WithRetries задаёт число повторных попыток для retryable ошибок
// (KindConnection, KindRateLimit). Значение 0 — без повторов.
func WithRetries(retries int) Option {
	return func(c *Client) {
		if retries > 0 {
			c.retries = retries
		}
	}
}

// NewClient создаёт OpenAI-compatible клиент.
// apiKey может быть пустым (локальные сервисы без авторизации).
func NewClient(baseURL, model, apiKey string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		apiKey:  apiKey,
		http:    &http.Client{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Generate выполняет запрос к /chat/completions и нормализует ошибки.
// Retryable ошибки повторяются WithRetries() раз с паузой, пока жив контекст.
func (c *Client) Generate(ctx context.Context, request GenerationRequest) (GenerationResponse, error) {
	attempts := c.retries + 1
	for attempt := 1; ; attempt++ {
		response, err := c.generateOnce(ctx, request)
		if err == nil || !Retryable(err) || attempt >= attempts {
			return response, err
		}

		timer := time.NewTimer(retryDelay(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return GenerationResponse{}, NewError(KindTimeout, 0, "повтор прерван контекстом", ctx.Err())
		case <-timer.C:
		}
	}
}

// retryDelay — экспоненциальная пауза между попытками: 300ms, 600ms, 1.2s, ...
func retryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return 300 * time.Millisecond
	}
	delay := 300 * time.Millisecond
	for i := 1; i < attempt && i < 5; i++ {
		delay *= 2
	}
	return delay
}

// generateOnce выполняет один запрос к /chat/completions.
func (c *Client) generateOnce(ctx context.Context, request GenerationRequest) (GenerationResponse, error) {
	model := request.Model
	if model == "" {
		model = c.model
	}

	body, err := c.buildRequestBody(model, request)
	if err != nil {
		return GenerationResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", body)
	if err != nil {
		return GenerationResponse{}, NewError(KindConnection, 0, "не удалось создать запрос", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return GenerationResponse{}, classifyTransportError(err, ctx)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return GenerationResponse{}, NewError(KindConnection, 0, "не удалось прочитать ответ", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return GenerationResponse{}, classifyHTTPError(resp.StatusCode, respBody)
	}

	return parseSuccess(respBody)
}

// buildRequestBody сериализует запрос в OpenAI-compatible формат.
func (c *Client) buildRequestBody(model string, request GenerationRequest) (*bytes.Buffer, error) {
	messages := make([]struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}, 0, len(request.Messages))
	for _, m := range request.Messages {
		messages = append(messages, struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{Role: string(m.Role), Content: m.Content})
	}

	payload := map[string]any{
		"model":    model,
		"messages": messages,
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.MaxTokens != nil {
		payload["max_tokens"] = *request.MaxTokens
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, NewError(KindInvalidJSON, 0, "не удалось сериализовать запрос", err)
	}
	return bytes.NewBuffer(data), nil
}

// classifyTransportError разбирает сетевые ошибки и таймауты.
func classifyTransportError(err error, ctx context.Context) error {
	if ctx != nil && ctx.Err() == context.DeadlineExceeded {
		return NewError(KindTimeout, 0, "превышен таймаут запроса", err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return NewError(KindTimeout, 0, "превышен таймаут запроса", err)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return NewError(KindConnection, 0, "ошибка соединения", err)
	}
	return NewError(KindConnection, 0, "ошибка соединения", err)
}

// classifyHTTPError классифицирует ответ с не-2xx статусом.
func classifyHTTPError(statusCode int, body []byte) error {
	kind := KindHTTP
	message := strings.TrimSpace(string(body))

	if statusCode == http.StatusTooManyRequests {
		kind = KindRateLimit
	}
	if statusCode == http.StatusBadRequest && isContextOverflowMessage(message) {
		kind = KindContextOverflow
	}

	return NewError(kind, statusCode, truncate(message), nil)
}

// parseSuccess разбирает успешный ответ.
func parseSuccess(body []byte) (GenerationResponse, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		return GenerationResponse{}, NewError(KindInvalidJSON, 0, "невалидный JSON ответа", err)
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return GenerationResponse{}, NewError(KindInvalidResponse, 0, "ответ не содержит текста: choices пуст или content пуст", nil)
	}

	return GenerationResponse{
		Content:      parsed.Choices[0].Message.Content,
		FinishReason: parsed.Choices[0].FinishReason,
		Usage: Usage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		},
	}, nil
}

// isContextOverflowMessage определяет переполнение контекста по сообщению ошибки.
func isContextOverflowMessage(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "context_length_exceeded") ||
		strings.Contains(lower, "maximum context length")
}

// truncate ограничивает длину сообщения об ошибке.
func truncate(s string) string {
	const maxLen = 512
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}