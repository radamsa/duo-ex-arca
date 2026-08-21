// Пакет debate — протокол и движок дебата двух LLM.
//
// Prompt builders (TASK-030..033) формируют системный и пользовательский
// промпты. Протокол ответов LLM — размеченный текст с секциями;
// разбор в доменные структуры выполняет textparse.go.
package debate

import (
	"fmt"
	"strings"

	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/llm"
)

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

// systemTextRule — общее требование к формату ответа.
const systemTextRule = "Строго придерживайся формата: ЗАГОЛОВОК: значение; элементы списков — с новой строки через дефис. Без markdown-разметки и комментариев вне формата."

// proposalFormat — формат ответа для предложения и пересмотра.
const proposalFormat = `Формат ответа:
РЕШЕНИЕ: <решение одной строкой>
АРГУМЕНТЫ:
- <аргумент>
ДОПУЩЕНИЯ:
- <допущение>
РИСКИ:
- <риск>
УВЕРЕННОСТЬ: <число от 0.0 до 1.0>

Пустые разделы можно не писать.`

// critiqueFormat — формат ответа для критики.
const critiqueFormat = `Формат ответа:
ВЕРНЫЕ УТВЕРЖДЕНИЯ:
- <верное утверждение оппонента>
ОШИБКИ:
- <ошибка>
НЕ ХВАТАЕТ ИНФОРМАЦИИ:
- <какой информации не хватает>
РИСКИ:
- <риск>
КОНТРАРГУМЕНТЫ:
- <контраргумент>
ПРЕДЛАГАЕМЫЕ ИЗМЕНЕНИЯ:
- <предлагаемое изменение>

Пустые разделы можно не писать.`

// consensusFormat — формат ответа арбитра.
const consensusFormat = `Формат ответа:
СОГЛАСИЕ: <CONSENSUS | DISAGREEMENT | INSUFFICIENT_DATA>
РЕШЕНИЕ: <главное решение одной короткой фразой (до 10 слов), без обоснований>
ТРЕБОВАНИЯ:
- <ключевое требование>
АРГУМЕНТЫ:
- <существенный аргумент>
РИСКИ:
- <критический риск>
УВЕРЕННОСТЬ: <число от 0.0 до 1.0>
ОБОСНОВАНИЕ: <краткое обоснование одной строкой>`

// ProposalPrompt формирует промпт для независимого предложения (TASK-030).
//
// contextText — подготовленный ContextBuilder-ом текст. На этом этапе
// в него НЕ попадает ответ другого участника (инвариант I5).
func ProposalPrompt(task domain.Task, contextText string) []llm.Message {
	system := strings.Join([]string{
		"Ты — участник дебата двух LLM. Предложи решение задачи.",
		proposalFormat,
		systemTextRule,
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
		critiqueFormat,
		systemTextRule,
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
		proposalFormat,
		systemTextRule,
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

// SimilarityPrompt формирует промпт проверки смыслового совпадения
// двух формулировок решения. Арбитр отвечает одним числом 0..1.
func SimilarityPrompt(decisionA, decisionB string) []llm.Message {
	system := strings.Join([]string{
		"Ты — арбитр дебата двух LLM. Оцени смысловое совпадение двух решений.",
		"Совпадение по смыслу означает одно и то же решение задачи, даже если формулировки различаются словами. Разные по сути решения (в том числе отличающиеся отрицанием) имеют низкий коэффициент.",
		"Ответь ТОЛЬКО одним числом от 0.0 до 1.0 — коэффициентом совпадения. Без слов, пояснений и других символов.",
	}, "\n")

	user := strings.Join([]string{
		"Решение 1: " + decisionA,
		"Решение 2: " + decisionB,
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
		consensusFormat,
		"Не выбирай победителя искусственно: если согласия нет, укажи DISAGREEMENT; если обеим сторонам не хватает данных — INSUFFICIENT_DATA.",
		systemTextRule,
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
