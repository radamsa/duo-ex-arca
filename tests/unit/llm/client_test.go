// Тесты OpenAI-compatible HTTP клиента через httptest.Server.
// Реальные LLM-сервисы в unit-тестах не используются.
package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/radamsa/duo-ex-arca/internal/llm"
)

// okResponse — корректный ответ OpenAI-compatible сервиса.
func okResponse() string {
	return `{
		"choices": [
			{
				"message": {"role": "assistant", "content": "Использовать SQLite"},
				"finish_reason": "stop"
			}
		],
		"usage": {"prompt_tokens": 10, "completion_tokens": 15, "total_tokens": 25}
	}`
}

// TestGenerateSuccess — клиент разбирает успешный ответ.
func TestGenerateSuccess(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("невалидный JSON запроса: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(okResponse()))
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String()+"/v1", "mock-model", "secret-key")
	resp, err := client.Generate(context.Background(), llm.GenerationRequest{
		Model: "mock-model",
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "Ты критик."},
			{Role: llm.RoleUser, Content: "Оцени решение."},
		},
	})
	if err != nil {
		t.Fatalf("Generate вернул ошибку: %v", err)
	}

	if resp.Content != "Использовать SQLite" {
		t.Fatalf("Content = %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 25 {
		t.Fatalf("Usage = %+v", resp.Usage)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, ожидался /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer secret-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}

	msgs, ok := gotBody["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("messages в запросе неверные: %+v", gotBody)
	}
}

// TestGenerateWithoutKey — при пустом ключе заголовок Authorization не отправляется.
func TestGenerateWithoutKey(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(okResponse()))
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "")
	if _, err := client.Generate(context.Background(), llm.GenerationRequest{}); err != nil {
		t.Fatalf("Generate вернул ошибку: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization отправлен без ключа: %q", gotAuth)
	}
}

// TestGenerateRateLimit — HTTP 429 классифицируется как rate_limit.
func TestGenerateRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": {"message": "too many requests"}}`))
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "")
	_, err := client.Generate(context.Background(), llm.GenerationRequest{})
	if llm.KindOf(err) != llm.KindRateLimit {
		t.Fatalf("Kind = %q, ожидался rate_limit (%v)", llm.KindOf(err), err)
	}
}

// TestGenerateHTTPError — прочие HTTP-ошибки классифицируются как http.
func TestGenerateHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": {"message": "internal error"}}`))
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "")
	_, err := client.Generate(context.Background(), llm.GenerationRequest{})
	if llm.KindOf(err) != llm.KindHTTP {
		t.Fatalf("Kind = %q, ожидался http (%v)", llm.KindOf(err), err)
	}

	var typed *llm.Error
	if !errors.As(err, &typed) || typed.StatusCode != http.StatusInternalServerError {
		t.Fatalf("StatusCode не сохранён: %+v", err)
	}
}

// TestGenerateContextOverflow — переполнение контекста распознаётся.
func TestGenerateContextOverflow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": {"message": "context_length_exceeded: too long"}}`))
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "")
	_, err := client.Generate(context.Background(), llm.GenerationRequest{})
	if llm.KindOf(err) != llm.KindContextOverflow {
		t.Fatalf("Kind = %q, ожидался context_overflow (%v)", llm.KindOf(err), err)
	}
}

// TestGenerateInvalidJSON — невалидный JSON тела ответа.
func TestGenerateInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{это не json`))
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "")
	_, err := client.Generate(context.Background(), llm.GenerationRequest{})
	if llm.KindOf(err) != llm.KindInvalidJSON {
		t.Fatalf("Kind = %q, ожидался invalid_json (%v)", llm.KindOf(err), err)
	}
}

// TestGenerateInvalidResponse — JSON валиден, но структура ответа неверная.
func TestGenerateInvalidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices": []}`))
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "")
	_, err := client.Generate(context.Background(), llm.GenerationRequest{})
	if llm.KindOf(err) != llm.KindInvalidResponse {
		t.Fatalf("Kind = %q, ожидался invalid_response (%v)", llm.KindOf(err), err)
	}
}

// TestGenerateConnectionError — недоступный сервер классифицируется как connection.
func TestGenerateConnectionError(t *testing.T) {
	client := llm.NewClient("http://127.0.0.1:1", "mock-model", "")
	_, err := client.Generate(context.Background(), llm.GenerationRequest{})
	if llm.KindOf(err) != llm.KindConnection {
		t.Fatalf("Kind = %q, ожидался connection (%v)", llm.KindOf(err), err)
	}
}

// TestGenerateTimeout — превышение таймаута классифицируется как timeout.
func TestGenerateTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := client.Generate(ctx, llm.GenerationRequest{})
	if llm.KindOf(err) != llm.KindTimeout {
		t.Fatalf("Kind = %q, ожидался timeout (%v)", llm.KindOf(err), err)
	}
}

// TestGenerateRequestBody — temperature и max_tokens пробрасываются в запрос.
func TestGenerateRequestBody(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("невалидный JSON запроса: %v", err)
		}
		_, _ = w.Write([]byte(okResponse()))
	}))
	defer srv.Close()

	temp := 0.7
	maxTokens := 512

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "")
	_, err := client.Generate(context.Background(), llm.GenerationRequest{
		Model:      "mock-model",
		Temperature: &temp,
		MaxTokens:  &maxTokens,
	})
	if err != nil {
		t.Fatalf("Generate вернул ошибку: %v", err)
	}
	if _, ok := gotBody["temperature"]; !ok {
		t.Fatal("temperature не передан в запрос")
	}
	if _, ok := gotBody["max_tokens"]; !ok {
		t.Fatal("max_tokens не передан в запрос")
	}
}

// TestGenerateEmptyContent — ответ без содержимого считается невалидным.
func TestGenerateEmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices": [{"message": {"role": "assistant", "content": ""}}]}`))
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "")
	_, err := client.Generate(context.Background(), llm.GenerationRequest{})
	if llm.KindOf(err) != llm.KindInvalidResponse {
		t.Fatalf("Kind = %q, ожидался invalid_response (%v)", llm.KindOf(err), err)
	}
}

// TestGenerateErrorBodyWithoutMessage — тело ошибки не JSON: категория http сохраняется.
func TestGenerateErrorBodyWithoutMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`ошибка шлюза`))
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "")
	_, err := client.Generate(context.Background(), llm.GenerationRequest{})
	if llm.KindOf(err) != llm.KindHTTP {
		t.Fatalf("Kind = %q, ожидался http (%v)", llm.KindOf(err), err)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("сообщение ошибки не содержит код статуса: %v", err)
	}
}

// TestGenerateRetry — retryable ошибка (429) повторяется и завершается успехом.
func TestGenerateRetry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": {"message": "rate limit"}}`))
			return
		}
		_, _ = w.Write([]byte(okResponse()))
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "", llm.WithRetries(3))
	response, err := client.Generate(context.Background(), llm.GenerationRequest{})
	if err != nil {
		t.Fatalf("ожидался успех после повторов: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("попыток = %d, ожидалось 3", attempts)
	}
	if !strings.Contains(response.Content, "SQLite") {
		t.Fatalf("содержимое ответа не разобрано: %q", response.Content)
	}
}

// TestGenerateRetryNoRetryForNonRetryable — HTTP 400 не повторяется.
func TestGenerateRetryNoRetryForNonRetryable(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": {"message": "bad request"}}`))
	}))
	defer srv.Close()

	client := llm.NewClient("http://"+srv.Listener.Addr().String(), "mock-model", "", llm.WithRetries(3))
	_, err := client.Generate(context.Background(), llm.GenerationRequest{})
	if llm.KindOf(err) != llm.KindHTTP {
		t.Fatalf("Kind = %q, ожидался http", llm.KindOf(err))
	}
	if attempts != 1 {
		t.Fatalf("попыток = %d, ожидалась 1", attempts)
	}
}