// Тесты построителей промптов и разбора JSON-ответов LLM.
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
	required := []string{"decision", "arguments", "assumptions", "risks", "confidence", "JSON"}
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
		"valid_points", "errors", "missing_information",
		"risks", "counter_arguments", "proposed_changes", "JSON",
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
		Errors:           []string{"нет оценки нагрузки"},
		ProposedChanges:  []string{"добавить оговорку про WAL"},
		MissingInformation: []string{"требования к конкурентности"},
	}

	msgs := debate.RevisionPrompt(task, own, critique, "контекст")
	sys := systemMessage(msgs)
	if !strings.Contains(strings.ToLower(sys), "пересмотр") {
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
		"agreement", "decision", "requirements", "arguments",
		"risks", "confidence", "JSON",
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

// --- Разбор Proposal JSON ---

const validProposalJSON = `{
	"decision": "Использовать SQLite",
	"arguments": ["ноль настроек", "один файл"],
	"assumptions": ["данные небольшие"],
	"risks": ["потеря файла"],
	"confidence": 0.9
}`

func TestParseProposal(t *testing.T) {
	p, err := debate.ParseProposal(validProposalJSON)
	if err != nil {
		t.Fatalf("ParseProposal вернул ошибку: %v", err)
	}
	if p.Decision != "Использовать SQLite" {
		t.Fatalf("Decision = %q", p.Decision)
	}
	if len(p.Arguments) != 2 || len(p.Risks) != 1 {
		t.Fatalf("списки не разобраны: %+v", p)
	}
	if p.Confidence != 0.9 {
		t.Fatalf("Confidence = %v", p.Confidence)
	}
}

// TestParseProposalDirty — модели часто добавляют пояснения вокруг JSON
// или оборачивают его в markdown-блоки; парсер должен извлекать объект.
func TestParseProposalDirty(t *testing.T) {
	cases := []string{
		"Вот моё предложение: " + validProposalJSON,
		"```json\n" + validProposalJSON + "\n```",
		"```\n" + validProposalJSON + "\n```",
		"Преамбула.\n" + validProposalJSON + "\nПостскриптум.",
		"{\"decision\": \"A\"}: пояснение после объекта",
		"{\\\"decision\\\":\\\"A\\\",\\\"arguments\\\":[],\\\"confidence\\\":0.5}",
		"Некоторые модели экранируют даже фигурные скобки: \\{\\\"decision\\\":\\\"A\\\"\\}",
	}
	for _, raw := range cases {
		p, err := debate.ParseProposal(raw)
		if err != nil {
			t.Fatalf("ParseProposal(%q) вернул ошибку: %v", raw, err)
		}
		if p.Decision == "" {
			t.Fatalf("решение не извлечено из %q", raw)
		}
	}
}

func TestParseProposalInvalid(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"не JSON", "не json"},
		{"пустое решение", `{"decision": "", "confidence": 0.5}`},
		{"нет решения", `{"arguments": ["a"]}`},
		{"уверенность вне диапазона", `{"decision": "x", "confidence": 2}`},
		{"уверенность отрицательная", `{"decision": "x", "confidence": -1}`},
		{"пустой JSON объект", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := debate.ParseProposal(tc.json); err == nil {
				t.Error("ожидалась ошибка разбора")
			}
		})
	}
}

// --- Разбор Critique JSON ---

const validCritiqueJSON = `{
	"valid_points": ["аргумент верен"],
	"errors": ["не указана нагрузка"],
	"missing_information": ["требования к конкурентности"],
	"risks": ["потеря данных"],
	"counter_arguments": ["можно PostgreSQL"],
	"proposed_changes": ["добавить WAL"]
}`

func TestParseCritique(t *testing.T) {
	c, err := debate.ParseCritique(validCritiqueJSON)
	if err != nil {
		t.Fatalf("ParseCritique вернул ошибку: %v", err)
	}
	if len(c.Errors) != 1 || len(c.ValidPoints) != 1 || len(c.ProposedChanges) != 1 {
		t.Fatalf("поля критики не разобраны: %+v", c)
	}
	if !c.HasContent() {
		t.Fatal("разобранная критика должна иметь содержимое")
	}
}

func TestParseCritiqueInvalid(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"не JSON", "не json"},
		{"пустая критика", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := debate.ParseCritique(tc.json); err == nil {
				t.Error("ожидалась ошибка разбора")
			}
		})
	}
}

// --- Разбор Consensus verdict JSON ---

const validVerdictJSON = `{
	"agreement": "CONSENSUS",
	"decision": "SQLite с WAL",
	"requirements": ["без C toolchain"],
	"arguments": ["низкая стоимость поддержки"],
	"risks": ["один файл — узкое место"],
	"confidence": 0.9,
	"reasoning": "обе стороны сошлись"
}`

func TestParseConsensusVerdict(t *testing.T) {
	v, err := debate.ParseConsensusVerdict(validVerdictJSON)
	if err != nil {
		t.Fatalf("ParseConsensusVerdict вернул ошибку: %v", err)
	}
	if v.Agreement != debate.AgreementConsensus {
		t.Fatalf("Agreement = %q", v.Agreement)
	}
	if len(v.Requirements) != 1 || v.Confidence != 0.9 {
		t.Fatalf("поля вердикта не разобраны: %+v", v)
	}
}

func TestParseConsensusVerdictInvalid(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"не JSON", "не json"},
		{"нет поля agreement", `{"decision": "x"}`},
		{"неизвестный agreement", `{"agreement": "MAYBE"}`},
		{"уверенность вне диапазона", `{"agreement": "CONSENSUS", "confidence": 7}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := debate.ParseConsensusVerdict(tc.json); err == nil {
				t.Error("ожидалась ошибка разбора")
			}
		})
	}
}

// --- Round-trip JSON ---

func TestProposalRoundTrip(t *testing.T) {
	// Кандидат должен разбираться из строки, которую сам же может отдать.
	p, err := domain.NewProposal("p-1", "participant-a", "Выбрать SQLite",
		[]string{"аргумент"}, []string{"допущение"}, []string{"риск"}, 0.7)
	if err != nil {
		t.Fatal(err)
	}
	raw := debate.ProposalToJSON(p)
	back, err := debate.ParseProposal(raw)
	if err != nil {
		t.Fatalf("обратный разбор не удался: %v", err)
	}
	if back.Decision != p.Decision || back.Confidence != p.Confidence {
		t.Fatalf("round-trip не совпал: %+v vs %+v", back, p)
	}
}