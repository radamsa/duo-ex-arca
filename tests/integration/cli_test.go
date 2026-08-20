// Интеграционные тесты CLI: полный pipeline CLI -> AgentRunner ->
// DebateEngine -> Mock LLM (httptest.Server) -> SQLite -> Decision.
//
// Реальные LLM-сервисы не используются: поднимаются два httptest-сервера,
// имитирующие OpenAI-compatible /chat/completions (docs/plan-mvp.md, TASK-140..141).
package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/storage/sqlite"
	"gopkg.in/yaml.v3"
)

// binPath — путь к собранному бинарнику hey-duo.
var binPath string

// TestMain собирает бинарник hey-duo один раз.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "hey-duo-int-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration: temp dir: ", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "hey-duo")
	cmd := exec.Command("go", "build", "-o", binPath, "github.com/radamsa/duo-ex-arca/cmd/hey-duo")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "integration: сборка бинарника: ", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// LLM phase и ответы — те же контракты, что в tests/integration/mockllm.

// phase — фаза протокола дебата.
type phase string

const (
	phaseProposal  phase = "proposal"
	phaseCritique  phase = "critique"
	phaseRevision  phase = "revision"
	phaseConsensus phase = "consensus"
)

// chatRequest — часть тела запроса Chat Completions.
type chatRequest struct {
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// llmHandler — httptest-обработчик OpenAI-compatible API.
// behave — режим имитации провайдера: "ok", "http500", "invalid_json".
func llmHandler(behave func(phase) (int, string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var system string
		for _, m := range req.Messages {
			if m.Role == "system" {
				system = m.Content
			}
		}
		status, content := behave(classifyPhase(system))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = w.Write([]byte(content))
			return
		}
		// Оборачиваем content в OpenAI-compatible конверт.
		envelope := fmt.Sprintf(`{"choices":[{"message":{"role":"assistant","content":%s},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":5,"total_tokens":10}}`, jsonContent(content))
		_, _ = w.Write([]byte(envelope))
	})
}

// jsonContent экранирует строку для встраивания в JSON.
func jsonContent(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(data)
}

// classifyPhase определяет фазу по системному промпту.
func classifyPhase(system string) phase {
	switch {
	case strings.Contains(system, "арбитр дебата"):
		return phaseConsensus
	case strings.Contains(system, "Пересмотри СВОЁ предложение"):
		return phaseRevision
	case strings.Contains(system, "Критически оцени предложение оппонента"):
		return phaseCritique
	default:
		return phaseProposal
	}
}

// okPhaseResponse — корректный ответ для фазы.
func okPhaseResponse(p phase) string {
	switch p {
	case phaseProposal:
		return `{"decision":"Использовать SQLite","arguments":["чистый Go"],"assumptions":["лицензия совместима"],"risks":["производительность"],"confidence":0.9}`
	case phaseCritique:
		return `{"valid_points":["чистый Go"],"errors":["нужны миграции"],"missing_information":["объём данных"],"risks":["блокировки"],"counter_arguments":["конкурентная запись"],"proposed_changes":["миграции"]}`
	case phaseRevision:
		return `{"decision":"Использовать SQLite","arguments":["чистый Go","миграции"],"assumptions":["лицензия совместима"],"risks":["блокировки"],"confidence":0.9}`
	default:
		return `{"agreement":"CONSENSUS","decision":"Использовать SQLite","requirements":["чистый Go"],"arguments":["миграции"],"risks":["блокировки"],"confidence":0.9,"reasoning":"согласны"}`
	}
}

// runCLI запускает бинарник hey-duo и возвращает stdout/stderr/exit code.
func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("запуск hey-duo: %v", err)
		}
	}
	return out.String(), code
}

// writeConfig создаёт конфигурацию, указывающую на два сервера.
func writeConfig(t *testing.T, path, urlA, urlB string) {
	t.Helper()
	cfg := fmt.Sprintf(`llm:
  participant_a: {base_url: %q, model: "mock-a"}
  participant_b: {base_url: %q, model: "mock-b"}
  timeout_seconds: 60
debate:
  default_mode: "normal"
  max_rounds: {normal: 1, deliberate: 3, critical: 6}
  consensus_threshold: 0.85
storage:
  type: "sqlite"
  path: %q
`, urlA, urlB, filepath.Join(filepath.Dir(path), "arca-int.db"))
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatalf("запись конфигурации: %v", err)
	}
}

// startLLMPair поднимает два LLM-сервера с одинаковым поведением.
func startLLMPair(t *testing.T, behave func(phase) (int, string)) (string, string) {
	t.Helper()
	serverA := httptest.NewServer(llmHandler(behave))
	serverB := httptest.NewServer(llmHandler(behave))
	t.Cleanup(serverA.Close)
	t.Cleanup(serverB.Close)
	return serverA.URL, serverB.URL
}

// TestCLIAskFast — полный pipeline в режиме FAST.
func TestCLIAskFast(t *testing.T) {
	urlA, urlB := startLLMPair(t, func(p phase) (int, string) {
		return http.StatusOK, okPhaseResponse(p)
	})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "arca.yaml")
	writeConfig(t, cfgPath, urlA, urlB)

	out, code := runCLI(t, "ask", "--config", cfgPath, "--mode", "fast", "--json", "Какую базу использовать?")
	if code != 0 {
		t.Fatalf("exit=%d, вывод: %s", code, out)
	}

	var result struct {
		TaskID string `json:"task_id"`
		Mode   string `json:"mode"`
		Decision struct {
			Status   string  `json:"status"`
			Decision string  `json:"decision"`
			Confidence float64 `json:"confidence"`
		} `json:"decision"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("вывод не JSON: %v\n%s", err, out)
	}
	if result.Decision.Status != "CONSENSUS" {
		t.Fatalf("status = %s, ожидался CONSENSUS", result.Decision.Status)
	}
	if result.Decision.Decision != "Использовать SQLite" {
		t.Fatalf("decision = %q", result.Decision.Decision)
	}
	if result.TaskID == "" {
		t.Fatal("пустой task_id")
	}
	if result.Mode != "FAST" {
		t.Fatalf("mode = %s, ожидался FAST", result.Mode)
	}
}

// TestCLIAskNormalFullPipeline — полный дебатный pipeline.
func TestCLIAskNormalFullPipeline(t *testing.T) {
	urlA, urlB := startLLMPair(t, func(p phase) (int, string) {
		return http.StatusOK, okPhaseResponse(p)
	})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "arca.yaml")
	writeConfig(t, cfgPath, urlA, urlB)

	out, code := runCLI(t, "ask", "--config", cfgPath, "--mode", "normal", "--json", "Какую базу использовать?")
	if code != 0 {
		t.Fatalf("exit=%d, вывод: %s", code, out)
	}
	if !strings.Contains(out, `"status": "CONSENSUS"`) {
		t.Fatalf("ожидался CONSENSUS: %s", out)
	}

	// trace должен содержать события движка.
	var result struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || result.TaskID == "" {
		t.Fatalf("task_id не извлечён: %v (%s)", err, out)
	}

	traceOut, code := runCLI(t, "trace", "--config", cfgPath, result.TaskID)
	if code != 0 {
		t.Fatalf("trace exit=%d: %s", code, traceOut)
	}
	for _, event := range []string{
		"TASK_CREATED", "CONTEXT_BUILT", "PROPOSAL_STARTED",
		"PROPOSAL_COMPLETED", "CRITIQUE_COMPLETED", "REVISION_COMPLETED",
		"CONSENSUS_EVALUATED", "DECISION_CREATED",
	} {
		if !strings.Contains(traceOut, event) {
			t.Fatalf("trace не содержит %s:\n%s", event, traceOut)
		}
	}
}

// Проверка конфигурации и health при работающих провайдерах.

// TestCLIConfigAndHealth — подкоманды config и health.
func TestCLIConfigAndHealth(t *testing.T) {
	urlA, urlB := startLLMPair(t, func(p phase) (int, string) {
		return http.StatusOK, okPhaseResponse(p)
	})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "arca.yaml")
	writeConfig(t, cfgPath, urlA, urlB)

	out, code := runCLI(t, "config", "--config", cfgPath)
	if code != 0 {
		t.Fatalf("config exit=%d: %s", code, out)
	}
	for _, token := range []string{"Участник A", "Участник B", "mock-a", "mock-b"} {
		if !strings.Contains(out, token) {
			t.Fatalf("config не содержит %q: %s", token, out)
		}
	}

	healthOut, code := runCLI(t, "health", "--config", cfgPath)
	if code != 0 {
		t.Fatalf("health exit=%d: %s", code, healthOut)
	}
	if !strings.Contains(healthOut, "доступен") {
		t.Fatalf("health не сообщает доступность: %s", healthOut)
	}
}

// Сценарии отказа (TASK-141): протокол не должен давать ложный consensus.

// TestCLIAskHTTP500 — HTTP 500 от провайдера: решение FAILED, exit != 0.
func TestCLIAskHTTP500(t *testing.T) {
	urlA, urlB := startLLMPair(t, func(p phase) (int, string) {
		_ = p
		return http.StatusInternalServerError, `{"error":{"message":"внутренняя ошибка"}}`
	})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "arca.yaml")
	writeConfig(t, cfgPath, urlA, urlB)

	out, code := runCLI(t, "ask", "--config", cfgPath, "--mode", "normal", "--json", "Какая база лучше?")
	if code == 0 {
		t.Fatalf("ожидался ненулевой exit при HTTP 500, вывод: %s", out)
	}
	if strings.Contains(out, "CONSENSUS") {
		t.Fatalf("ложный консенсус при отказе провайдера: %s", out)
	}
}

// TestCLIAskInvalidJSON — невалидный JSON ответа: решение FAILED, не consensus.
func TestCLIAskInvalidJSON(t *testing.T) {
	urlA, urlB := startLLMPair(t, func(p phase) (int, string) {
		return http.StatusOK, `это не JSON`
	})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "arca.yaml")
	writeConfig(t, cfgPath, urlA, urlB)

	out, code := runCLI(t, "ask", "--config", cfgPath, "--mode", "normal", "--json", "Какая база лучше?")
	if code == 0 {
		t.Fatalf("ожидался ненулевой exit при невалидном JSON, вывод: %s", out)
	}
	if strings.Contains(out, "CONSENSUS") {
		t.Fatalf("ложный консенсус при невалидном JSON: %s", out)
	}
}

// TestCLIDisagreement — честное разногласие арбитра: DISAGREEMENT
// штатный результат акта, команда завершается с кодом 0.
func TestCLIDisagreement(t *testing.T) {
	urlA, urlB := startLLMPair(t, func(p phase) (int, string) {
		if p == phaseConsensus {
			return http.StatusOK, `{"agreement":"DISAGREEMENT","decision":"","requirements":[],"arguments":[],"risks":[],"confidence":0.5,"reasoning":"модели не согласны"}`
		}
		return http.StatusOK, okPhaseResponse(p)
	})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "arca.yaml")
	writeConfig(t, cfgPath, urlA, urlB)

	out, code := runCLI(t, "ask", "--config", cfgPath, "--mode", "normal", "--json", "Какая база лучше?")
	if code != 0 {
		t.Fatalf("DISAGREEMENT должен быть штатным результатом, exit=%d: %s", code, out)
	}
	if !strings.Contains(out, "DISAGREEMENT") {
		t.Fatalf("ожидался DISAGREEMENT: %s", out)
	}
	if strings.Contains(out, "CONSENSUS") {
		t.Fatalf("ложный консенсус при разногласии: %s", out)
	}
}

// writeDataset создаёт JSONL-датасет для бенчмарка.
func writeDataset(t *testing.T, path string, items []string) {
	t.Helper()
	content := strings.Join(items, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("запись датасета: %v", err)
	}
}

// TestCLIBenchFast — бенчмарк в режиме FAST (baseline, TASK-151):
// каждая задача — один вызов LLM, решение и метрики сохраняются в SQLite.
func TestCLIBenchFast(t *testing.T) {
	urlA, urlB := startLLMPair(t, func(p phase) (int, string) {
		return http.StatusOK, okPhaseResponse(p)
	})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "arca.yaml")
	writeConfig(t, cfgPath, urlA, urlB)

	// mock отвечает "Использовать SQLite" на любой запрос:
	// задача 001 совпадает с ожиданием, 002 — нет.
	datasetPath := filepath.Join(dir, "dataset.jsonl")
	writeDataset(t, datasetPath, []string{
		`{"id":"001","category":"storage","task":"Какую БД использовать?","expected":"Использовать SQLite"}`,
		`{"id":"002","category":"language","task":"Какой язык?","expected":"Go"}`,
	})

	out, code := runCLI(t, "bench", "--config", cfgPath, "--dataset", datasetPath, "--mode", "fast")
	if code != 0 {
		t.Fatalf("bench exit=%d: %s", code, out)
	}
	if !strings.Contains(out, "Итог") {
		t.Fatalf("bench не вывел итог: %s", out)
	}
	// 001 -> 1.00, 002 -> 0.00: меткость 0.50.
	if !strings.Contains(out, "меткость 0.50") {
		t.Fatalf("ожидался итог с меткостью 0.50: %s", out)
	}

	// Прогоны сохранены в SQLite (TASK-153).
	verifyBenchRows(t, cfgPath, 2)
}

// TestCLIBenchNormal — duo-дебат: раунды учитываются, метрики сохраняются.
func TestCLIBenchNormal(t *testing.T) {
	urlA, urlB := startLLMPair(t, func(p phase) (int, string) {
		return http.StatusOK, okPhaseResponse(p)
	})

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "arca.yaml")
	writeConfig(t, cfgPath, urlA, urlB)

	datasetPath := filepath.Join(dir, "dataset.jsonl")
	writeDataset(t, datasetPath, []string{
		`{"id":"001","category":"storage","task":"Какую БД использовать?","expected":"Использовать SQLite"}`,
	})

	out, code := runCLI(t, "bench", "--config", cfgPath, "--dataset", datasetPath, "--mode", "normal")
	if code != 0 {
		t.Fatalf("bench exit=%d: %s", code, out)
	}
	// В дебате — один полный раунд (proposal+critique+revision+consensus).
	if !strings.Contains(out, "меткость 1.00") {
		t.Fatalf("ожидался итог с меткостью 1.00: %s", out)
	}
	// В нормальном режиме один раунд.
	if !strings.Contains(out, "1 ") || !strings.Contains(out, "NORMAL") {
		if !strings.Contains(out, "rounds") && !strings.Contains(out, "1") {
			t.Fatalf("не найден раунд в выводе: %s", out)
		}
	}

	verifyBenchRows(t, cfgPath, 1)
}

// verifyBenchRows проверяет, что прогоны сохранены в benchmark_runs.
func verifyBenchRows(t *testing.T, cfgPath string, want int) {
	t.Helper()

	// Читаем путь БД из конфигурации (сторадж в том же каталоге).
	var cfg struct {
		Storage struct {
			Path string `yaml:"path"`
		} `yaml:"storage"`
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("чтение конфигурации: %v", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("разбор конфигурации: %v", err)
	}

	db, err := sqlite.Open(cfg.Storage.Path)
	if err != nil {
		t.Fatalf("открытие БД: %v", err)
	}
	defer db.Close()

	rows := db.QueryRow(`SELECT COUNT(*) FROM benchmark_runs`)
	var count int
	if err := rows.Scan(&count); err != nil {
		t.Fatalf("подсчёт прогонов: %v", err)
	}
	if count != want {
		t.Fatalf("прогонов в benchmark_runs = %d, ожидалось %d", count, want)
	}
}