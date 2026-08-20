// Пакет main — протокол-aware mock LLM-сервер для интеграционных тестов.
//
// Отвечает на любой POST /chat/completions JSON-ответом, соответствующим
// фазе дебата: proposal/critique/revision/consensus. Фаза определяется
// по системному сообщению промпта, сформированного internal/debate/prompts.go.
//
// Используется только в тестах (tests/integration) и локальной отладке;
// в продукт не входит.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// phase — фаза протокола дебата.
type phase string

const (
	phaseProposal  phase = "proposal"
	phaseCritique  phase = "critique"
	phaseRevision  phase = "revision"
	phaseConsensus phase = "consensus"
	phasePing      phase = "ping"
)

// chatRequest — достаточная часть тела запроса Chat Completions.
type chatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func main() {
	port := os.Args[1]
	http.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, "невалидный запрос")
			return
		}

		var system string
		for _, m := range req.Messages {
			if m.Role == "system" {
				system = m.Content
			}
		}

		content := respondFor(system)
		writeJSON(w, map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"role": "assistant", "content": content},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10},
		})
	})
	fmt.Println("mock LLM слушает :" + port)
	http.ListenAndServe("127.0.0.1:"+port, nil)
}

// respondFor формирует JSON-ответ по системному промпту.
func respondFor(system string) string {
	switch classify(system) {
	case phaseProposal:
		return `{"decision":"Использовать SQLite","arguments":["чистый Go","без C toolchain"],"assumptions":["есть вендор"],"risks":["производительность"],"confidence":0.9}`
	case phaseCritique:
		return `{"valid_points":["чистый Go"],"errors":["нет миграций"],"missing_information":["объём данных"],"risks":["блокировки"],"counter_arguments":["SQLite не для конкурентной записи"],"proposed_changes":["добавить миграции"]}`
	case phaseRevision:
		return `{"decision":"Использовать SQLite","arguments":["чистый Go","без C toolchain","миграции добавлены"],"assumptions":["есть вендор"],"risks":["блокировки"],"confidence":0.9}`
	case phaseConsensus:
		return `{"agreement":"CONSENSUS","decision":"Использовать SQLite","requirements":["чистый Go","без C toolchain"],"arguments":["нативная интеграция","миграции"],"risks":["блокировки при конкурентной записи"],"confidence":0.9,"reasoning":"модели согласны по решению"}`
	default: // phasePing или неизвестная фаза
		return `{"decision":"ок","arguments":[],"assumptions":[],"risks":[],"confidence":0.5}`
	}
}

// classify определяет фазу по содержимому системного промпта.
func classify(system string) phase {
	switch {
	case strings.Contains(system, "арбитр дебата"):
		return phaseConsensus
	case strings.Contains(system, "Пересмотри СВОЁ предложение"):
		return phaseRevision
	case strings.Contains(system, "Критически оцени предложение оппонента"):
		return phaseCritique
	case strings.Contains(system, "Предложи решение задачи"):
		return phaseProposal
	case strings.Contains(system, "Ответь одним словом"):
		return phasePing
	default:
		return phaseProposal
	}
}

// writeJSON пишет ответ в JSON.
func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// httpError пишет HTTP-ошибку.
func httpError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	writeJSON(w, map[string]any{"error": map[string]any{"message": message, "code": strconv.Itoa(code)}})
}