// Тесты построителей промптов и разбора текстовых ответов LLM.
package debate_test

import (
	"strings"
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/debate"
	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/llm"
)

func mustTask(t *testing.T) domain.Task {
	t.Helper()
	task, err := domain.NewTask("t-1", "Какую БД выбрать?", "Нужна СУБД для Go-проекта", []string{"только Go"}, domain.NORMAL)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func mustProposal(t *testing.T, decision, participant string) domain.Proposal {
	t.Helper()
	p, err := domain.NewProposal("p-1", participant, decision,
		[]string{"простота"}, []string{"переезд не нужен"}, []string{"потеря файла"}, 0.85)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// allText объединяет содержимое сообщений для поиска подстрок.
func allText(msgs []llm.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(m.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}

// systemMessage возвращает первое сообщение с ролью system.
func systemMessage(msgs []llm.Message) string {
	for _, m := range msgs {
		if m.Role == llm.RoleSystem {
			return m.Content
		}
	}
	return ""
}

// --- Proposal prompt (TASK-030) ---

func TestProposalPrompt(t *testing.T) {
	task := mustTask(t)
	const contextText = "Контекст: участник A, общих данных нет."

	msgs := debate.ProposalPrompt(task, contextText)
	if len(msgs) != 2 {
		t.Fatalf("ожидалось 2 сообщения, получено %d", len(msgs))
	}

	sys := systemMessage(msgs)
	required := []string{"РЕШЕНИЕ:", "АРГУМЕНТЫ:", "ДОПУЩЕНИЯ:", "РИСКИ:", "УВЕРЕННОСТЬ:"}
	for _, field := range required {
		if !strings.Contains(sys, field) {
			t.Errorf("системный промпт не упоминает %q", field)
		}
	}

	user := allText(msgs)
	if !strings.Contains(user, task.Title) {
		t.Error("промпт не содержит заголовка задачи")
	}
	if !strings.Contains(user, contextText) {
		t.Error("промпт не содержит контекста")
	}
}

// --- Critique prompt (TASK-031) ---

func TestCritiquePrompt(t *testing.T) {
	task := mustTask(t)
	target := mustProposal(t, "Использовать SQLite", "participant-b")

	msgs := debate.CritiquePrompt(task, target, "контекст")
	sys := systemMessage(msgs)
	required := []string{
		"ВЕРНЫЕ УТВЕРЖДЕНИЯ:", "ОШИБКИ:", "НЕ ХВАТАЕТ ИНФОРМАЦИИ:",
		"РИСКИ:", "КОНТРАРГУМЕНТЫ:", "ПРЕДЛАГАЕМЫЕ ИЗМЕНЕНИЯ:",
	}
	for _, field := range required {
		if !strings.Contains(sys, field) {
			t.Errorf("системный промпт критики не упоминает %q", field)
		}
	}

	user := allText(msgs)
	if !strings.Contains(user, "Использовать SQLite") {
		t.Error("промпт критики не содержит предложения оппонента")
	}
}

// --- Revision prompt (TASK-032) ---

func TestRevisionPrompt(t *testing.T) {
	task := mustTask(t)
	own := mustProposal(t, "Использовать SQLite", "participant-a")
	critique := domain.Critique{
		Errors:             []string{"нет оценки нагрузки"},
		ProposedChanges:    []string{"добавить оговорку про WAL"},
		MissingInformation: []string{"требования к конкурентности"},
	}

	msgs := debate.RevisionPrompt(task, own, critique, "контекст")
	sys := systemMessage(msgs)
	if !strings.Contains(strings.ToLower(sys), "пересмотри") {
		t.Error("системный промпт пересмотра не идентифицирует фазу")
	}

	user := allText(msgs)
	if !strings.Contains(user, "Использовать SQLite") {
		t.Error("промпт пересмотра не содержит собственного предложения")
	}
	if !strings.Contains(user, "нет оценки нагрузки") {
		t.Error("промпт пересмотра не содержит критики")
	}
}

// --- Consensus prompt (TASK-033) ---

func TestConsensusPrompt(t *testing.T) {
	pa := mustProposal(t, "SQLite", "participant-a")
	pb := mustProposal(t, "SQLite", "participant-b")
	ra := mustProposal(t, "SQLite с WAL", "participant-a")
	rb := mustProposal(t, "SQLite с WAL", "participant-b")
	requirements := []string{"без C toolchain"}

	msgs := debate.ConsensusPrompt(pa, pb, ra, rb, requirements)
	sys := systemMessage(msgs)
	required := []string{
		"СОГЛАСИЕ:", "РЕШЕНИЕ:", "ТРЕБОВАНИЯ:", "АРГУМЕНТЫ:",
		"РИСКИ:", "УВЕРЕННОСТЬ:", "ОБОСНОВАНИЕ:",
	}
	for _, field := range required {
		if !strings.Contains(sys, field) {
			t.Errorf("системный промпт консенсуса не упоминает %q", field)
		}
	}

	user := allText(msgs)
	for _, fragment := range []string{"SQLite", "SQLite с WAL", "без C toolchain"} {
		if !strings.Contains(user, fragment) {
			t.Errorf("промпт консенсуса не содержит %q", fragment)
		}
	}
}

// --- Разбор Proposal (текстовый протокол) ---

const validProposalText = `РЕШЕНИЕ: Использовать SQLite
АРГУМЕНТЫ:
- ноль настроек
- один файл
ДОПУЩЕНИЯ:
- данные небольшие
РИСКИ:
- потеря файла
УВЕРЕННОСТЬ: 0.9`

func TestParseProposal(t *testing.T) {
	p, err := debate.ParseProposal(validProposalText)
	if err != nil {
		t.Fatalf("ParseProposal вернул ошибку: %v", err)
	}
	if p.Decision != "Использовать SQLite" {
		t.Fatalf("Decision = %q", p.Decision)
	}
	if len(p.Arguments) != 2 || len(p.Risks) != 1 || len(p.Assumptions) != 1 {
		t.Fatalf("списки не разобраны: %+v", p)
	}
	if p.Confidence != 0.9 {
		t.Fatalf("Confidence = %v", p.Confidence)
	}
}

// TestParseProposalDirty — модели пишут заголовки в разных стилях;
// парсер должен извлекать секции из «грязного» текста.
func TestParseProposalDirty(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		decision string
	}{
		{"markdown-жирный", "**РЕШЕНИЕ:** Использовать SQLite\n**АРГУМЕНТЫ:**\n- простота", "Использовать SQLite"},
		{"нумерация", "1. РЕШЕНИЕ: Использовать SQLite\n2. АРГУМЕНТЫ:\n- простота", "Использовать SQLite"},
		{"английские заголовки", "DECISION: Use SQLite\nARGUMENTS:\n- simple\nCONFIDENCE: 0.8", "Use SQLite"},
		{"преамбула перед ответом", "Вот моё предложение.\nРЕШЕНИЕ: Использовать SQLite\nУВЕРЕННОСТЬ: 0.7", "Использовать SQLite"},
		{"звёздочки-маркеры", "РЕШЕНИЕ: Использовать SQLite\nАРГУМЕНТЫ:\n* простота\n* один файл", "Использовать SQLite"},
		{"решение на строке заголовка отсутствует", "РЕШЕНИЕ:\nИспользовать SQLite с WAL\nУВЕРЕННОСТЬ: 0.6", "Использовать SQLite с WAL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := debate.ParseProposal(tc.raw)
			if err != nil {
				t.Fatalf("ParseProposal вернул ошибку: %v", err)
			}
			if p.Decision != tc.decision {
				t.Fatalf("Decision = %q, ожидалось %q", p.Decision, tc.decision)
			}
		})
	}
}

// TestParseProposalDefaults — пропущенные секции заменяются значениями
// по умолчанию, а не приводят к ошибке.
func TestParseProposalDefaults(t *testing.T) {
	p, err := debate.ParseProposal("РЕШЕНИЕ: Использовать SQLite")
	if err != nil {
		t.Fatalf("ParseProposal: %v", err)
	}
	if len(p.Arguments) != 0 || len(p.Risks) != 0 || len(p.Assumptions) != 0 {
		t.Fatalf("пропущенные списки должны быть пустыми: %+v", p)
	}
	if p.Confidence != 0.5 {
		t.Fatalf("Confidence по умолчанию = %v, ожидалось 0.5", p.Confidence)
	}
}

// TestParseProposalConfidenceFormats — уверенность в разных форматах.
func TestParseProposalConfidenceFormats(t *testing.T) {
	cases := []struct {
		raw  string
		want float64
	}{
		{"РЕШЕНИЕ: x\nУВЕРЕННОСТЬ: 0,8", 0.8},
		{"РЕШЕНИЕ: x\nУВЕРЕННОСТЬ: 90%", 0.9},
		{"РЕШЕНИЕ: x\nCONFIDENCE: 85", 0.85},
		{"РЕШЕНИЕ: x\nУВЕРЕННОСТЬ: нет", 0.5},
	}
	for _, tc := range cases {
		p, err := debate.ParseProposal(tc.raw)
		if err != nil {
			t.Fatalf("ParseProposal(%q): %v", tc.raw, err)
		}
		if diff := p.Confidence - tc.want; diff < -0.001 || diff > 0.001 {
			t.Fatalf("%q: Confidence = %v, ожидалось %v", tc.raw, p.Confidence, tc.want)
		}
	}
}

// TestParseProposalFallbackDecision — если секции РЕШЕНИЕ нет вовсе,
// решением становится первая содержательная строка ответа.
func TestParseProposalFallbackDecision(t *testing.T) {
	p, err := debate.ParseProposal("Предлагаю использовать SQLite из-за простоты.")
	if err != nil {
		t.Fatalf("ParseProposal: %v", err)
	}
	if p.Decision == "" {
		t.Fatal("решение не извлечено из свободного текста")
	}
}

// TestParseProposalInvalid — совсем пустой ответ — ошибка.
func TestParseProposalInvalid(t *testing.T) {
	cases := []string{"", "   \n\t\n"}
	for _, raw := range cases {
		if _, err := debate.ParseProposal(raw); err == nil {
			t.Errorf("ожидалась ошибка для ответа %q", raw)
		}
	}
}

// --- Разбор Critique ---

const validCritiqueText = `ВЕРНЫЕ УТВЕРЖДЕНИЯ:
- выбор SQLite обоснован
ОШИБКИ:
- нет оценки конкурентности
НЕ ХВАТАЕТ ИНФОРМАЦИИ:
- требования к нагрузке
РИСКИ:
- блокировки записи
КОНТРАРГУМЕНТЫ:
- Postgres даёт репликацию
ПРЕДЛАГАЕМЫЕ ИЗМЕНЕНИЯ:
- добавить WAL`

func TestParseCritique(t *testing.T) {
	c, err := debate.ParseCritique(validCritiqueText)
	if err != nil {
		t.Fatalf("ParseCritique вернул ошибку: %v", err)
	}
	if len(c.ValidPoints) != 1 || len(c.Errors) != 1 || len(c.MissingInformation) != 1 ||
		len(c.Risks) != 1 || len(c.CounterArguments) != 1 || len(c.ProposedChanges) != 1 {
		t.Fatalf("критика разобрана неполностью: %+v", c)
	}
	if c.Errors[0] != "нет оценки конкурентности" {
		t.Fatalf("Errors[0] = %q", c.Errors[0])
	}
}

// TestParseCritiquePartial — частичная критика валидна.
func TestParseCritiquePartial(t *testing.T) {
	c, err := debate.ParseCritique("ОШИБКИ:\n- нет оценки нагрузки")
	if err != nil {
		t.Fatalf("ParseCritique: %v", err)
	}
	if len(c.Errors) != 1 || c.HasContent() == false {
		t.Fatalf("критика не разобрана: %+v", c)
	}
}

// TestParseCritiqueInvalid — без содержательных секций критика отклоняется.
func TestParseCritiqueInvalid(t *testing.T) {
	cases := []string{"", "просто текст без секций"}
	for _, raw := range cases {
		if _, err := debate.ParseCritique(raw); err == nil {
			t.Errorf("ожидалась ошибка для ответа %q", raw)
		}
	}
}

// --- Разбор ConsensusVerdict ---

const validVerdictText = `СОГЛАСИЕ: CONSENSUS
РЕШЕНИЕ: Использовать SQLite
ТРЕБОВАНИЯ:
- чистый Go
АРГУМЕНТЫ:
- миграции просты
РИСКИ:
- блокировки
УВЕРЕННОСТЬ: 0.9
ОБОСНОВАНИЕ: участники согласны`

func TestParseConsensusVerdict(t *testing.T) {
	v, err := debate.ParseConsensusVerdict(validVerdictText)
	if err != nil {
		t.Fatalf("ParseConsensusVerdict вернул ошибку: %v", err)
	}
	if v.Agreement != debate.AgreementConsensus {
		t.Fatalf("Agreement = %s", v.Agreement)
	}
	if v.Decision != "Использовать SQLite" {
		t.Fatalf("Decision = %q", v.Decision)
	}
	if len(v.Requirements) != 1 || len(v.Arguments) != 1 || len(v.Risks) != 1 {
		t.Fatalf("списки не разобраны: %+v", v)
	}
	if v.Confidence != 0.9 {
		t.Fatalf("Confidence = %v", v.Confidence)
	}
	if v.Reasoning != "участники согласны" {
		t.Fatalf("Reasoning = %q", v.Reasoning)
	}
}

// TestParseConsensusVerdictAgreements — все три значения согласия,
// включая русские синонимы и грязный формат.
func TestParseConsensusVerdictAgreements(t *testing.T) {
	cases := []struct {
		raw  string
		want debate.Agreement
	}{
		{"СОГЛАСИЕ: CONSENSUS\nРЕШЕНИЕ: x", debate.AgreementConsensus},
		{"СОГЛАСИЕ: DISAGREEMENT\nРЕШЕНИЕ: x", debate.AgreementDisagreement},
		{"СОГЛАСИЕ: INSUFFICIENT_DATA\nРЕШЕНИЕ: x", debate.AgreementInsufficientData},
		{"Согласие: консенсус достигнут", debate.AgreementConsensus},
		{"**СОГЛАСИЕ:** несогласие по хранению", debate.AgreementDisagreement},
		{"AGREEMENT: insufficient data", debate.AgreementInsufficientData},
	}
	for _, tc := range cases {
		v, err := debate.ParseConsensusVerdict(tc.raw)
		if err != nil {
			t.Fatalf("ParseConsensusVerdict(%q): %v", tc.raw, err)
		}
		if v.Agreement != tc.want {
			t.Fatalf("%q: Agreement = %s, ожидалось %s", tc.raw, v.Agreement, tc.want)
		}
	}
}

// TestParseConsensusVerdictDefaultInsufficient — нераспознанное согласие
// трактуется как INSUFFICIENT_DATA, а не как искусственный консенсус.
func TestParseConsensusVerdictDefaultInsufficient(t *testing.T) {
	v, err := debate.ParseConsensusVerdict("Не могу определить.")
	if err != nil {
		t.Fatalf("ParseConsensusVerdict: %v", err)
	}
	if v.Agreement != debate.AgreementInsufficientData {
		t.Fatalf("Agreement = %s, ожидался INSUFFICIENT_DATA", v.Agreement)
	}
}

// --- Сериализация в текстовый протокол (для mock и тестов) ---

func TestProposalToTextRoundTrip(t *testing.T) {
	src := domain.Proposal{
		Decision:    "SQLite",
		Arguments:   []string{"простота"},
		Assumptions: []string{"малые данные"},
		Risks:       []string{"блокировки"},
		Confidence:  0.75,
	}
	p, err := debate.ParseProposal(debate.ProposalToText(src))
	if err != nil {
		t.Fatalf("ParseProposal(ProposalToText): %v", err)
	}
	if p.Decision != src.Decision || len(p.Arguments) != 1 || len(p.Risks) != 1 || len(p.Assumptions) != 1 {
		t.Fatalf("round-trip искажён: %+v", p)
	}
	if diff := p.Confidence - src.Confidence; diff < -0.01 || diff > 0.01 {
		t.Fatalf("round-trip Confidence = %v", p.Confidence)
	}
}

func TestCritiqueToTextRoundTrip(t *testing.T) {
	src := domain.Critique{Errors: []string{"x"}, ProposedChanges: []string{"y"}}
	c, err := debate.ParseCritique(debate.CritiqueToText(src))
	if err != nil {
		t.Fatalf("ParseCritique(CritiqueToText): %v", err)
	}
	if len(c.Errors) != 1 || len(c.ProposedChanges) != 1 {
		t.Fatalf("round-trip искажён: %+v", c)
	}
}

func TestVerdictToTextRoundTrip(t *testing.T) {
	src := debate.ConsensusVerdict{
		Agreement:  debate.AgreementDisagreement,
		Decision:   "нет единого решения",
		Confidence: 0.4,
		Reasoning:  "подходы различаются",
	}
	v, err := debate.ParseConsensusVerdict(debate.VerdictToText(src))
	if err != nil {
		t.Fatalf("ParseConsensusVerdict(VerdictToText): %v", err)
	}
	if v.Agreement != src.Agreement || v.Decision != src.Decision || v.Reasoning != src.Reasoning {
		t.Fatalf("round-trip искажён: %+v", v)
	}
}
