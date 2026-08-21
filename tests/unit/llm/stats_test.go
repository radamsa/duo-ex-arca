// Тесты сборщика статистики вызовов LLM.
package llm_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/radamsa/duo-ex-arca/internal/llm"
)

// TestStatsCollectorAccumulates — токены и время накапливаются по участникам.
func TestStatsCollectorAccumulates(t *testing.T) {
	collector := llm.NewStatsCollector()
	mock := llm.NewMock()
	mock.Respond("a1", llm.Usage{PromptTokens: 100, CompletionTokens: 20})
	mock.Respond("a2", llm.Usage{PromptTokens: 50, CompletionTokens: 30})

	client := llm.WithStats("participant-a", mock, collector)
	for i := 0; i < 2; i++ {
		if _, err := client.Generate(context.Background(), llm.GenerationRequest{}); err != nil {
			t.Fatalf("Generate вернул ошибку: %v", err)
		}
	}

	s := collector.Snapshot("participant-a")
	if s.Requests != 2 {
		t.Fatalf("Requests = %d, ожидалось 2", s.Requests)
	}
	if s.PromptTokens != 150 || s.CompletionTokens != 50 {
		t.Fatalf("токены: вход %d / выход %d, ожидалось 150/50", s.PromptTokens, s.CompletionTokens)
	}
	if s.TotalTokens() != 200 {
		t.Fatalf("TotalTokens = %d, ожидалось 200", s.TotalTokens())
	}
	if s.TotalDuration <= 0 || s.LastDuration <= 0 {
		t.Fatalf("время не измерено: total=%v last=%v", s.TotalDuration, s.LastDuration)
	}
	if s.LastDuration > time.Second {
		t.Fatalf("LastDuration подозрительно велик: %v", s.LastDuration)
	}
}

// TestStatsCollectorErrorCountsTime — неудачный запрос учитывает время,
// но не токены.
func TestStatsCollectorErrorCountsTime(t *testing.T) {
	collector := llm.NewStatsCollector()
	mock := llm.NewMock()
	mock.Fail(errors.New("модель недоступна"))

	client := llm.WithStats("participant-b", mock, collector)
	if _, err := client.Generate(context.Background(), llm.GenerationRequest{}); err == nil {
		t.Fatal("ожидалась ошибка от мока")
	}

	s := collector.Snapshot("participant-b")
	if s.Requests != 1 {
		t.Fatalf("Requests = %d, ожидался 1 (неудачный вызов тоже запрос)", s.Requests)
	}
	if s.TotalTokens() != 0 {
		t.Fatalf("при ошибке токены не учитываются, получено %d", s.TotalTokens())
	}
	if s.TotalDuration <= 0 {
		t.Fatal("время неудачного запроса должно учитываться")
	}
}

// TestStatsCollectorNil — WithStats с nil-сборщиком возвращает inner как есть.
func TestStatsCollectorNil(t *testing.T) {
	mock := llm.NewMock()
	mock.Respond("ок", llm.Usage{})
	if got := llm.WithStats("x", mock, nil); got != llm.LLM(mock) {
		t.Fatal("WithStats с nil-сборщиком должен вернуть inner без обёртки")
	}
}

// TestStatsCollectorConcurrent — параллельные вызовы не гоняются
// (запускать с -race).
func TestStatsCollectorConcurrent(t *testing.T) {
	collector := llm.NewStatsCollector()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "participant-" + string(rune('a'+i%2))
			mock := llm.NewMock()
			mock.Respond("ок", llm.Usage{PromptTokens: 10, CompletionTokens: 5})
			client := llm.WithStats(id, mock, collector)
			for j := 0; j < 10; j++ {
				_, _ = client.Generate(context.Background(), llm.GenerationRequest{})
			}
		}(i)
	}
	wg.Wait()

	a := collector.Snapshot("participant-a")
	b := collector.Snapshot("participant-b")
	if a.Requests+b.Requests != 80 {
		t.Fatalf("потеряны вызовы: a=%d b=%d, ожидалось 80 суммарно", a.Requests, b.Requests)
	}
}

// TestStatsAll — All возвращает копию по всем участникам.
func TestStatsAll(t *testing.T) {
	collector := llm.NewStatsCollector()
	mock := llm.NewMock()
	mock.Respond("ок", llm.Usage{PromptTokens: 7, CompletionTokens: 3})
	if _, err := llm.WithStats("participant-a", mock, collector).Generate(context.Background(), llm.GenerationRequest{}); err != nil {
		t.Fatal(err)
	}

	all := collector.All()
	s, ok := all["participant-a"]
	if !ok {
		t.Fatal("All не содержит participant-a")
	}
	if s.TotalSeconds <= 0 {
		t.Fatalf("TotalSeconds не заполнено: %+v", s)
	}
	if s.LastSeconds <= 0 {
		t.Fatalf("LastSeconds не заполнено: %+v", s)
	}
}
