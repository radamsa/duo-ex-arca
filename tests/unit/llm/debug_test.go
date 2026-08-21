// Тесты отладочного декоратора: полные промпты и ответы пишутся в writer.
package llm_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/llm"
)

// TestDebugClientLogsRequestAndResponse — декоратор пишет промпт и ответ целиком.
func TestDebugClientLogsRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ПОЛНЫЙ-ОТВЕТ-МОДЕЛИ"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer server.Close()

	var buf bytes.Buffer
	inner := llm.NewClient(server.URL, "test-model", "")
	debug := llm.NewDebugClient("participant-a", &buf, inner)

	resp, err := debug.Generate(context.Background(), llm.GenerationRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "СИСТЕМНЫЙ-ПРОМПТ"},
			{Role: llm.RoleUser, Content: "ПОЛЬЗОВАТЕЛЬСКИЙ-ВОПРОС"},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "ПОЛНЫЙ-ОТВЕТ-МОДЕЛИ" {
		t.Fatalf("ответ искажён декоратором: %q", resp.Content)
	}

	out := buf.String()
	for _, want := range []string{
		"participant-a",
		"запрос",
		"СИСТЕМНЫЙ-ПРОМПТ",
		"ПОЛЬЗОВАТЕЛЬСКИЙ-ВОПРОС",
		"ответ",
		"ПОЛНЫЙ-ОТВЕТ-МОДЕЛИ",
		"15 токенов",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("в логе нет %q:\n%s", want, out)
		}
	}
}

// TestDebugClientLogsError — ошибка провайдера попадает в лог.
func TestDebugClientLogsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer server.Close()

	var buf bytes.Buffer
	debug := llm.NewDebugClient("participant-b", &buf, llm.NewClient(server.URL, "m", ""))

	if _, err := debug.Generate(context.Background(), llm.GenerationRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "привет"}},
	}); err == nil {
		t.Fatal("ожидалась ошибка от сервера")
	}

	out := buf.String()
	if !strings.Contains(out, "ошибка") || !strings.Contains(out, "participant-b") {
		t.Errorf("в логе нет записи об ошибке:\n%s", out)
	}
}
