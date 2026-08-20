// Пакет debate — протокол и движок дебата двух LLM.
//
// Prompt builders (TASK-030..033) формируют системный и пользовательский
// промпты, а protocol.go описывает JSON-контракт ответов LLM:
// Proposal, Critique, ConsensusVerdict (docs/plan-mvp.md, TASK-030..033).
package debate

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/llm"
)

// proposalJSON — wire-формат предложения (он же schema в промпте).
type proposalJSON struct {
	Decision    string   `json:"decision"`
	Arguments   []string `json:"arguments"`
	Assumptions []string `json:"assumptions"`
	Risks       []string `json:"risks"`
	Confidence  float64  `json:"confidence"`
}

// critiqueJSON — wire-формат критики (TASK-031).
type critiqueJSON struct {
	ValidPoints        []string `json:"valid_points"`
	Errors             []string `json:"errors"`
	MissingInformation []string `json:"missing_information"`
	Risks              []string `json:"risks"`
	CounterArguments   []string `json:"counter_arguments"`
	ProposedChanges    []string `json:"proposed_changes"`
}

// Agreement — результат оценки консенсуса LLM-ом.
type Agreement string

const (
	// AgreementConsensus — модели согласны.
	AgreementConsensus Agreement = "CONSENSUS"
	// AgreementDisagreement — модели не согласны.
	AgreementDisagreement Agreement = "DISAGREEMENT"
	// AgreementInsufficientData — данных недостаточно.
	AgreementInsufficientData Agreement = "INSUFFICIENT_DATA"
)

func (a Agreement) Valid() bool {
	switch a {
	case AgreementConsensus, AgreementDisagreement, AgreementInsufficientData:
		return true
	default:
		return false
	}
}

// ConsensusVerdict — структурированная оценка консенсуса (TASK-033/TASK-061).
type ConsensusVerdict struct {
	Agreement Agreement `json:"agreement"`

	Decision       string   `json:"decision"`
	Requirements   []string `json:"requirements"`
	Arguments      []string `json:"arguments"`
	Risks          []string `json:"risks"`
	Confidence     float64  `json:"confidence"`
	Reasoning      string   `json:"reasoning"`
}

// systemJSONRule — общее требование выдавать только JSON.
const systemJSONRule = "Отвечай ТОЛЬКО валидным JSON без комментариев, markdown-разметки и пояснений."

// ProposalPrompt формирует промпт для независимого предложения (TASK-030).
//
// contextText — подготовленный ContextBuilder-ом текст. На этом этапе
// в него НЕ попадает ответ другого участника (инвариант I5).
func ProposalPrompt(task domain.Task, contextText string) []llm.Message {
	system := strings.Join([]string{
		"Ты — участник дебата двух LLM. Предложи решение задачи.",
		"Верни JSON со следующими полями:",
		`"decision" — решение (строка),`,
		`"arguments" — аргументы в пользу решения (массив строк),`,
		`"assumptions" — допущения (массив строк),`,
		`"risks" — риски (массив строк),`,
		`"confidence" — уверенность от 0.0 до 1.0 (число).`,
		systemJSONRule,
	}, "\n")

	user := strings.Join([]string{
		"Задача: " + task.Title,
		"Описание: " + task.Description,
		"Ограничения: " + strings.Join(task.Constraints, "; "),
		contextText,
	}, "\n")

	return []llm.Message{
		{Role: llm.RoleSystem, Content: system},
		{Role: llm.RoleUser, Content: user},
	}
}

// CritiquePrompt формирует промпт для критики предложения оппонента (TASK-031).
func CritiquePrompt(task domain.Task, target domain.Proposal, contextText string) []llm.Message {
	system := strings.Join([]string{
		"Ты — участник дебата двух LLM. Критически оцени предложение оппонента.",
		"Найди причины, по которым это решение может быть ошибочным или неполным.",
		"Верни JSON со следующими полями:",
		`"valid_points" — верные утверждения оппонента (массив строк),`,
		`"errors" — ошибки (массив строк),`,
		`"missing_information" — недостающая информация (массив строк),`,
		`"risks" — риски (массив строк),`,
		`"counter_arguments" — контраргументы (массив строк),`,
		`"proposed_changes" — предлагаемые изменения (массив строк).`,
		systemJSONRule,
	}, "\n")

	var args []string
	for _, a := range target.Arguments {
		args = append(args, "- "+a)
	}
	var risks []string
	for _, r := range target.Risks {
		risks = append(risks, "- "+r)
	}

	user := strings.Join([]string{
		"Задача: " + task.Title,
		"Предложение оппонента:",
		"Решение: " + target.Decision,
		"Аргументы:",
		strings.Join(args, "\n"),
		"Риски:",
		strings.Join(risks, "\n"),
		contextText,
	}, "\n")

	return []llm.Message{
		{Role: llm.RoleSystem, Content: system},
		{Role: llm.RoleUser, Content: user},
	}
}

// RevisionPrompt формирует промпт для пересмотра своего предложения (TASK-032).
func RevisionPrompt(task domain.Task, own domain.Proposal, critique domain.Critique, contextText string) []llm.Message {
	system := strings.Join([]string{
		"Ты — участник дебата двух LLM. Пересмотри СВОЁ предложение с учётом критики.",
		"Учти обоснованные замечания критики, но не отказывайся от решения без причины.",
		"Верни JSON в том же формате, что и исходное предложение:",
		`"decision", "arguments", "assumptions", "risks", "confidence".`,
		systemJSONRule,
	}, "\n")

	var critiqueLines []string
	for _, e := range critique.Errors {
		critiqueLines = append(critiqueLines, "- ошибка: "+e)
	}
	for _, m := range critique.MissingInformation {
		critiqueLines = append(critiqueLines, "- не хватает: "+m)
	}
	for _, c := range critique.ProposedChanges {
		critiqueLines = append(critiqueLines, "- предлагается: "+c)
	}
	for _, ca := range critique.CounterArguments {
		critiqueLines = append(critiqueLines, "- контраргумент: "+ca)
	}

	user := strings.Join([]string{
		"Задача: " + task.Title,
		"Твоё предложение: " + own.Decision,
		"Полученная критика:",
		strings.Join(critiqueLines, "\n"),
		contextText,
	}, "\n")

	return []llm.Message{
		{Role: llm.RoleSystem, Content: system},
		{Role: llm.RoleUser, Content: user},
	}
}

// ConsensusPrompt формирует промпт для оценки консенсуса (TASK-033).
func ConsensusPrompt(proposalA, proposalB, revisionA, revisionB domain.Proposal, requirements []string) []llm.Message {
	system := strings.Join([]string{
		"Ты — арбитр дебата двух LLM. Оцени, достигли ли участники консенсуса.",
		"Консенсус — это согласие по решению, ключевым требованиям, существенным аргументам и критическим рискам, а НЕ совпадение текстов.",
		"Верни JSON со следующими полями:",
		`"agreement" — одна из строк: "CONSENSUS", "DISAGREEMENT" или "INSUFFICIENT_DATA" (если обеим сторонам не хватает данных),`,
		`"decision" — согласованное решение (строка),`,
		`"requirements" — ключевые требования (массив строк),`,
		`"arguments" — существенные аргументы (массив строк),`,
		`"risks" — критические риски (массив строк),`,
		`"confidence" — уверенность в оценке от 0.0 до 1.0 (число),`,
		`"reasoning" — краткое обоснование (строка).`,
		"Не выбирай победителя искусственно: если согласия нет, верни DISAGREEMENT.",
		systemJSONRule,
	}, "\n")

	formatProposal := func(label string, p domain.Proposal) string {
		return strings.Join([]string{
			label + ":",
			"  решение: " + p.Decision,
			"  аргументы: " + strings.Join(p.Arguments, "; "),
			"  риски: " + strings.Join(p.Risks, "; "),
			"  уверенность: " + fmt.Sprintf("%.2f", p.Confidence),
		}, "\n")
	}

	user := strings.Join([]string{
		"Ключевые требования: " + strings.Join(requirements, "; "),
		formatProposal("Исходное предложение A", proposalA),
		formatProposal("Исходное предложение B", proposalB),
		formatProposal("Пересмотр A", revisionA),
		formatProposal("Пересмотр B", revisionB),
	}, "\n")

	return []llm.Message{
		{Role: llm.RoleSystem, Content: system},
		{Role: llm.RoleUser, Content: user},
	}
}

// ---------------------------------------------------------------------------
// Разбор JSON-ответов LLM (протокол)

// ParseProposal разбирает JSON-ответ LLM в доменную структуру Proposal.
// ID и ParticipantID проставляет движок — модель их не возвращает.
func ParseProposal(raw string) (domain.Proposal, error) {
	var parsed proposalJSON
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return domain.Proposal{}, fmt.Errorf("debate: невалидный JSON предложения: %w", err)
	}
	if strings.TrimSpace(parsed.Decision) == "" {
		return domain.Proposal{}, fmt.Errorf("debate: предложение не содержит решения")
	}
	if math.IsNaN(parsed.Confidence) || parsed.Confidence < 0 || parsed.Confidence > 1 {
		return domain.Proposal{}, fmt.Errorf("debate: уверенность %v вне диапазона [0,1]", parsed.Confidence)
	}
	return domain.Proposal{
		Decision:    parsed.Decision,
		Arguments:   parsed.Arguments,
		Assumptions: parsed.Assumptions,
		Risks:       parsed.Risks,
		Confidence:  parsed.Confidence,
	}, nil
}

// ParseCritique разбирает JSON-ответ LLM в доменную структуру Critique.
func ParseCritique(raw string) (domain.Critique, error) {
	var parsed critiqueJSON
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return domain.Critique{}, fmt.Errorf("debate: невалидный JSON критики: %w", err)
	}
	c := domain.Critique{
		ValidPoints:        parsed.ValidPoints,
		Errors:             parsed.Errors,
		MissingInformation: parsed.MissingInformation,
		Risks:              parsed.Risks,
		CounterArguments:   parsed.CounterArguments,
		ProposedChanges:    parsed.ProposedChanges,
	}
	if !c.HasContent() {
		return domain.Critique{}, fmt.Errorf("debate: критика пуста")
	}
	return c, nil
}

// ParseConsensusVerdict разбирает JSON-ответ арбитра.
func ParseConsensusVerdict(raw string) (ConsensusVerdict, error) {
	var parsed ConsensusVerdict
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ConsensusVerdict{}, fmt.Errorf("debate: невалидный JSON вердикта: %w", err)
	}
	if !parsed.Agreement.Valid() {
		return ConsensusVerdict{}, fmt.Errorf("debate: неизвестное значение agreement %q", parsed.Agreement)
	}
	if math.IsNaN(parsed.Confidence) || parsed.Confidence < 0 || parsed.Confidence > 1 {
		return ConsensusVerdict{}, fmt.Errorf("debate: уверенность %v вне диапазона [0,1]", parsed.Confidence)
	}
	return parsed, nil
}

// ProposalToJSON сериализует предложение в wire-формат (для тестов и trace).
func ProposalToJSON(p domain.Proposal) string {
	data, err := json.Marshal(proposalJSON{
		Decision:    p.Decision,
		Arguments:   p.Arguments,
		Assumptions: p.Assumptions,
		Risks:       p.Risks,
		Confidence:  p.Confidence,
	})
	if err != nil {
		return "{}"
	}
	return string(data)
}

// CritiqueToJSON сериализует критику в wire-формат.
func CritiqueToJSON(c domain.Critique) string {
	data, err := json.Marshal(critiqueJSON{
		ValidPoints:        c.ValidPoints,
		Errors:             c.Errors,
		MissingInformation: c.MissingInformation,
		Risks:              c.Risks,
		CounterArguments:   c.CounterArguments,
		ProposedChanges:    c.ProposedChanges,
	})
	if err != nil {
		return "{}"
	}
	return string(data)
}