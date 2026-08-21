package debate

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/radamsa/duo-ex-arca/internal/domain"
)

// ConsensusEngine объединяет два вердикта участников в итоговое решение.
//
// Правила (docs/plan-mvp.md, TASK-060..063; docs/design.md §14):
//   - CONSENSUS + CONSENSUS с одним и тем же решением (по смыслу,
//     а не побайтово) и уверенностью не ниже порога -> итог CONSENSUS;
//   - CONSENSUS + CONSENSUS с разными решениями -> DISAGREEMENT
//     (ложный консенсус запрещён);
//   - INSUFFICIENT_DATA + INSUFFICIENT_DATA -> INSUFFICIENT_DATA;
//   - любое другое сочетание -> DISAGREEMENT.
type ConsensusEngine struct {
	threshold float64
}

// NewConsensusEngine создаёт движок с порогом уверенности из (0,1].
func NewConsensusEngine(threshold float64) (*ConsensusEngine, error) {
	if threshold <= 0 || threshold > 1 {
		return nil, fmt.Errorf("debate: порог консенсуса %v вне диапазона (0,1]", threshold)
	}
	return &ConsensusEngine{threshold: threshold}, nil
}

// Evaluate вычисляет итоговое решение по двум вердиктам.
func (e *ConsensusEngine) Evaluate(v1, v2 ConsensusVerdict) (domain.Decision, error) {
	if v1.Agreement == AgreementConsensus && v2.Agreement == AgreementConsensus {
		return e.evaluateConsensus(v1, v2)
	}
	if v1.Agreement == AgreementInsufficientData && v2.Agreement == AgreementInsufficientData {
		return domain.NewDecision(domain.InsufficientData, "", minConfidence(v1, v2))
	}
	return domain.NewDecision(domain.Disagreement, "", minConfidence(v1, v2))
}

// evaluateConsensus — оба участника объявили консенсус.
func (e *ConsensusEngine) evaluateConsensus(v1, v2 ConsensusVerdict) (domain.Decision, error) {
	d1 := strings.TrimSpace(v1.Decision)
	d2 := strings.TrimSpace(v2.Decision)
	if d1 == "" || d2 == "" {
		return domain.Decision{}, fmt.Errorf("debate: вердикт консенсуса без текста решения")
	}

	confidence := minConfidence(v1, v2)
	if !sameDecision(d1, d2) {
		// Участники согласились с разными решениями — ложный консенсус,
		// искусственный выбор победителя запрещён (инвариант I6).
		return domain.NewDecision(domain.Disagreement, "", confidence)
	}
	if confidence < e.threshold {
		return domain.NewDecision(domain.Disagreement, "", confidence)
	}

	// Формулировки совпадают по смыслу; в итог берём более подробную.
	chosen := d1
	if utf8.RuneCountInString(d2) > utf8.RuneCountInString(d1) {
		chosen = d2
	}
	decision, err := domain.NewDecision(domain.Consensus, chosen, confidence)
	if err != nil {
		return domain.Decision{}, err
	}
	decision.SupportingArguments = union(v1.Arguments, v2.Arguments)
	decision.Risks = union(v1.Risks, v2.Risks)
	return decision, nil
}

// decisionSimilarityThreshold — минимальная доля общих значимых слов
// между двумя формулировками одного решения (коэффициент перекрытия
// |A∩B| / min(|A|,|B|)).
const decisionSimilarityThreshold = 0.7

// decisionStopwords — русские служебные слова, не влияющие на смысл
// решения. Отрицание «не» сознательно НЕ входит в список: «делать X»
// и «не делать X» — разные решения.
var decisionStopwords = map[string]bool{
	"и": true, "или": true, "либо": true, "но": true, "а": true,
	"однако": true, "что": true, "чтобы": true, "как": true,
	"это": true, "этот": true, "эта": true, "эти": true,
	"является": true, "являются": true,
	"быть": true, "был": true, "была": true, "было": true, "были": true,
	"будет": true, "будут": true, "есть": true,
	"в": true, "во": true, "на": true, "с": true, "со": true,
	"из": true, "от": true, "до": true, "по": true, "за": true,
	"под": true, "над": true, "при": true, "о": true, "об": true,
	"обо": true, "к": true, "ко": true, "у": true, "для": true,
	"без": true, "через": true, "между": true, "перед": true,
	"также": true, "тоже": true, "же": true, "бы": true, "ли": true,
	"да": true, "ну": true, "вот": true, "именно": true,
}

// sameDecision определяет, выражают ли две формулировки одно решение
// (design.md §14: консенсус — согласие по решению, а не совпадение
// текстов). Сначала тексты сравниваются после нормализации; если они
// различаются — по доле общих значимых слов.
func sameDecision(a, b string) bool {
	wa, wb := normalizeWords(a), normalizeWords(b)
	if strings.Join(wa, " ") == strings.Join(wb, " ") {
		return true
	}
	sa, sb := meaningfulSet(wa), meaningfulSet(wb)
	if len(sa) == 0 || len(sb) == 0 {
		return false
	}
	common := 0
	for w := range sa {
		if sb[w] {
			common++
		}
	}
	minLen := len(sa)
	if len(sb) < minLen {
		minLen = len(sb)
	}
	return float64(common)/float64(minLen) >= decisionSimilarityThreshold
}

// normalizeWords приводит текст к нижнему регистру, заменяет ё на е,
// выбрасывает знаки препинания и разбивает на слова.
func normalizeWords(s string) []string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

// meaningfulSet оставляет значимые слова: без служебных и слишком
// коротких токенов (отрицание «не» сохраняется всегда).
func meaningfulSet(words []string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		if w == "не" || (utf8.RuneCountInString(w) >= 3 && !decisionStopwords[w]) {
			set[w] = true
		}
	}
	return set
}

// minConfidence — консервативная уверенность итога.
func minConfidence(v1, v2 ConsensusVerdict) float64 {
	if v1.Confidence < v2.Confidence {
		return v1.Confidence
	}
	return v2.Confidence
}

// union объединяет списки, сохраняя порядок появления и без дублей.
func union(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	var result []string
	for _, item := range append(append([]string{}, a...), b...) {
		if seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}