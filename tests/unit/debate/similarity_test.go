// Тесты фазы смыслового совпадения: разбор коэффициента и промпт арбитра.
package debate_test

import (
	"strings"
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/debate"
)

// TestParseSimilarity — допустимые форматы ответа арбитра.
func TestParseSimilarity(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"0.95", 0.95},
		{"0,85", 0.85},
		{" 1.0 ", 1.0},
		{"0", 0},
		{"90%", 0.9},
		{"Коэффициент: 0.7", 0.7},
	}
	for _, c := range cases {
		got, err := debate.ParseSimilarity(c.in)
		if err != nil {
			t.Errorf("ParseSimilarity(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSimilarity(%q) = %v, ожидалось %v", c.in, got, c.want)
		}
	}
}

// TestParseSimilarityInvalid — невалидные ответы дают ошибку.
func TestParseSimilarityInvalid(t *testing.T) {
	for _, in := range []string{"", "не число", "совпадают полностью", "1.5", "abc123def"} {
		if _, err := debate.ParseSimilarity(in); err == nil {
			t.Errorf("ParseSimilarity(%q) должен вернуть ошибку", in)
		}
	}
}

// TestSimilarityPrompt — промпт содержит обе формулировки и требование
// ответить одним числом.
func TestSimilarityPrompt(t *testing.T) {
	msgs := debate.SimilarityPrompt("решение первое", "решение второе")

	if len(msgs) != 2 {
		t.Fatalf("ожидалось 2 сообщения, получено %d", len(msgs))
	}
	var system, user string
	for _, m := range msgs {
		switch m.Role {
		case "system":
			system = m.Content
		case "user":
			user = m.Content
		}
	}
	if !strings.Contains(system, "одним числом") {
		t.Fatalf("системный промпт не требует числовой ответ: %q", system)
	}
	if !strings.Contains(user, "решение первое") || !strings.Contains(user, "решение второе") {
		t.Fatalf("промпт должен содержать обе формулировки: %q", user)
	}
}
