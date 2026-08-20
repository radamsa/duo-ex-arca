// Тесты бенчмарк-харнесса: датасет, эвалуатор, подсчёт токенов.
package bench_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/bench"
	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/llm"
)

// writeTempDataset пишет JSONL-датасет во временный файл.
func writeTempDataset(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dataset.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadDataset — корректный датасет читается целиком.
func TestLoadDataset(t *testing.T) {
	path := writeTempDataset(t, `{"id":"001","category":"reasoning","task":"Вопрос?","expected":"Ответ"}
{"id":"002","task":"Без категории и ожидания"}
`)
	items, err := bench.LoadDataset(path)
	if err != nil {
		t.Fatalf("LoadDataset: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("задач = %d, ожидалось 2", len(items))
	}
	if items[0].ID != "001" || items[0].Expected != "Ответ" {
		t.Fatalf("элемент 0 разобран неверно: %+v", items[0])
	}
}

// TestLoadDatasetSkipsEmptyLines — пустые строки пропускаются.
func TestLoadDatasetSkipsEmptyLines(t *testing.T) {
	path := writeTempDataset(t, "\n\n{\"id\":\"001\",\"task\":\"Вопрос?\"}\n\n")
	items, err := bench.LoadDataset(path)
	if err != nil {
		t.Fatalf("LoadDataset: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("задач = %d, ожидалась 1", len(items))
	}
}

// TestLoadDatasetErrors — битый JSON и пустой id дают ошибку.
func TestLoadDatasetErrors(t *testing.T) {
	if _, err := bench.LoadDataset("/нет/такого/файла"); err == nil {
		t.Fatal("ожидалась ошибка для отсутствующего файла")
	}

	path := writeTempDataset(t, `{"id":"001","task":"ок"}
это не json
`)
	if _, err := bench.LoadDataset(path); err == nil {
		t.Fatal("ожидалась ошибка для невалидной строки")
	}

	path = writeTempDataset(t, `{"task":"без id"}
`)
	if _, err := bench.LoadDataset(path); err == nil {
		t.Fatal("ожидалась ошибка для элемента без id")
	}
}

// TestEvaluatorExactMatch — точное совпадение нормализованных текстов.
func TestEvaluatorExactMatch(t *testing.T) {
	evaluator := bench.NewEvaluator()
	decision, _ := domain.NewDecision(domain.Consensus, "Использовать SQLite", 0.9)

	if score := evaluator.Score("Использовать SQLite", decision); score != 1 {
		t.Fatalf("score = %v, ожидалась 1", score)
	}
	// Регистр и лишние пробелы не влияют.
	if score := evaluator.Score("  использовать sqlite ", decision); score != 1 {
		t.Fatalf("score = %v после нормализации, ожидалась 1", score)
	}
}

// TestEvaluatorMismatch — различие текстов даёт 0.
func TestEvaluatorMismatch(t *testing.T) {
	evaluator := bench.NewEvaluator()
	decision, _ := domain.NewDecision(domain.Consensus, "Использовать PostgreSQL", 0.9)
	if score := evaluator.Score("Использовать SQLite", decision); score != 0 {
		t.Fatalf("score = %v, ожидалась 0", score)
	}
}

// TestEvaluatorNoExpected — пустое ожидание даёт -1 (не оценивается).
func TestEvaluatorNoExpected(t *testing.T) {
	evaluator := bench.NewEvaluator()
	decision, _ := domain.NewDecision(domain.Consensus, "Любой ответ", 0.9)
	if score := evaluator.Score("", decision); score != -1 {
		t.Fatalf("score = %v, ожидалась -1", score)
	}
}

// TestEvaluatorNonConsensus — не-консенсусные статусы считаются неверными.
func TestEvaluatorNonConsensus(t *testing.T) {
	evaluator := bench.NewEvaluator()
	disagreement, _ := domain.NewDecision(domain.Disagreement, "", 0.5)
	if score := evaluator.Score("Использовать SQLite", disagreement); score != 0 {
		t.Fatalf("score = %v, ожидалась 0 для DISAGREEMENT", score)
	}
}

// countingLLM — тестовая LLM с известным usage.
type countingLLM struct {
	usage llm.Usage
}

// Generate возвращает фиксированный ответ с usage.
func (c *countingLLM) Generate(_ context.Context, _ llm.GenerationRequest) (llm.GenerationResponse, error) {
	return llm.GenerationResponse{Content: "{}", Usage: c.usage}, nil
}

// TestInstrumentCountsTokens — обёртка суммирует токены ответов.
func TestInstrumentCountsTokens(t *testing.T) {
	counter := bench.NewTokenCounter()
	decorated := bench.Instrument(&countingLLM{
		usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, counter)

	ctx := context.Background()
	resp, err := decorated.Generate(ctx, llm.GenerationRequest{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Fatalf("TotalTokens = %d, ожидалось 15", resp.Usage.TotalTokens)
	}
	if got := counter.Total(); got != 15 {
		t.Fatalf("counter.Total() = %d, ожидалось 15", got)
	}

	// Повторный вызов и Reset.
	if _, err := decorated.Generate(ctx, llm.GenerationRequest{}); err != nil {
		t.Fatal(err)
	}
	if got := counter.Total(); got != 30 {
		t.Fatalf("counter.Total() = %d, ожидалось 30", got)
	}
	counter.Reset()
	if got := counter.Total(); got != 0 {
		t.Fatalf("после Reset counter.Total() = %d, ожидалось 0", got)
	}
}

// TestInstrumentConcurrent — параллельные вызовы не теряют токены.
func TestInstrumentConcurrent(t *testing.T) {
	counter := bench.NewTokenCounter()
	decorated := bench.Instrument(&countingLLM{usage: llm.Usage{PromptTokens: 1, CompletionTokens: 1}}, counter)

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = decorated.Generate(ctx, llm.GenerationRequest{})
		}()
	}
	wg.Wait()
	if got := counter.Total(); got != 100 {
		t.Fatalf("counter.Total() = %d, ожидалось 100 (2 токена x 50 вызовов)", got)
	}
}