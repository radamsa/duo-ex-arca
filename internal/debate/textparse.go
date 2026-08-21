// Текстовый протокол ответов LLM: вместо JSON модель отвечает размеченным
// текстом с заголовками секций (РЕШЕНИЕ:, АРГУМЕНТЫ:, ...).
// Парсер устойчив к «грязному» выводу слабых моделей: регистр, markdown,
// нумерация, пропущенные секции (подставляются значения по умолчанию).
package debate

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/radamsa/duo-ex-arca/internal/domain"
)

// labelAliases — канонические ключи секций и их возможные заголовки.
// При сопоставлении алиасы проверяются от длинного к короткому,
// чтобы «предлагаемые изменения» не съедались ключом «изменения».
var labelAliases = map[string][]string{
	"decision":           {"решение", "decision", "ответ", "итоговый ответ", "заголовок"},
	"arguments":          {"аргументы", "arguments"},
	"assumptions":        {"допущения", "предположения", "assumptions"},
	"risks":              {"риски", "risks"},
	"confidence":         {"уверенность", "confidence"},
	"agreement":          {"согласие", "agreement", "вердикт"},
	"requirements":       {"требования", "requirements"},
	"reasoning":          {"обоснование", "reasoning", "пояснение"},
	"valid_points":       {"верные утверждения", "сильные стороны", "valid points", "верно", "достоинства"},
	"errors":             {"ошибки", "errors", "неверно", "проблемы"},
	"missing_information": {"не хватает информации", "недостающая информация", "missing information"},
	"counter_arguments":  {"контраргументы", "возражения", "counter arguments", "counterarguments"},
	"proposed_changes":   {"предлагаемые изменения", "предлагаемые правки", "proposed changes"},
}

// sortedAliases — все алиасы одним списком, от длинного к короткому.
var sortedAliases = buildSortedAliases()

func buildSortedAliases() []struct {
	key   string
	alias string
} {
	all := []struct {
		key   string
		alias string
	}{}
	for key, aliases := range labelAliases {
		for _, a := range aliases {
			all = append(all, struct {
				key   string
				alias string
			}{key, normalizeLabel(a)})
		}
	}
	// Сортировка по убыванию длины: сначала специфичные заголовки.
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if len(all[j].alias) > len(all[i].alias) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	return all
}

// document — разобранный ответ модели: преамбула до первого заголовка
// и секции в порядке появления.
type document struct {
	preamble []string
	order    []string
	sections map[string]*section
}

// section — заголовок, значение на строке заголовка и тело до следующего заголовка.
type section struct {
	inline string
	body   []string
}

// homoglyphs — латинские буквы, визуально совпадающие с кириллицей.
// Слабые модели часто смешивают алфавиты («VERНЫЕ УТВЕРЖДЕНИЯ»).
var homoglyphs = map[rune]rune{
	'a': 'а', 'b': 'в', 'c': 'с', 'e': 'е', 'h': 'н', 'i': 'и',
	'k': 'к', 'm': 'м', 'o': 'о', 'p': 'р', 'r': 'р', 's': 'с',
	't': 'т', 'u': 'у', 'v': 'в', 'x': 'х', 'y': 'у',
	'ı': 'и', 'і': 'и',
}

// normalizeLabel приводит строку к виду для сопоставления заголовков:
// нижний регистр, ё -> е, латинские омоглифы -> кириллица,
// схлопывание пробелов, удаление лишних символов.
func normalizeLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	var b strings.Builder
	for _, r := range s {
		if repl, ok := homoglyphs[r]; ok {
			r = repl
		}
		// Оставляем только буквы, цифры и пробелы.
		switch {
		case r == ' ' || (r >= 'а' && r <= 'я') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// levenshtein — расстояние редактирования с отсечкой по максимуму.
func levenshtein(a, b string, max int) int {
	ra, rb := []rune(a), []rune(b)
	if abs(len(ra)-len(rb)) > max {
		return max + 1
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// stripDecoration снимает markdown-разметку и нумерацию с начала строки,
// а также разметку с конца: "**1. РЕШЕНИЕ:**" -> "РЕШЕНИЕ:".
func stripDecoration(line string) string {
	s := strings.TrimSpace(line)
	trimmed := true
	for trimmed {
		trimmed = false
		s2 := strings.TrimLeft(s, "#>*`•–—- \t")
		if s2 != s {
			s, trimmed = s2, true
		}
		// Нумерация вида "1." или "1)".
		if idx := strings.IndexAny(s, ".)"); idx > 0 && idx <= 3 && isDigits(s[:idx]) {
			s = strings.TrimSpace(s[idx+1:])
			trimmed = true
		}
		s3 := strings.TrimRight(s, "#>*`: \t")
		if s3 != s {
			s, trimmed = s3, true
		}
	}
	return strings.TrimSpace(s)
}

// isDigits проверяет, что строка состоит только из цифр.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// matchHeader распознаёт строку-заголовок секции.
// Заголовок — это название секции (возможно, с двоеточием) в начале строки:
// «РЕШЕНИЕ: x», «Риски:», но не «Решение задачи требует анализа».
// Значение после двоеточия сохраняет оригинальный регистр.
func matchHeader(line string) (string, string, bool) {
	dec := stripDecoration(line)
	if dec == "" {
		return "", "", false
	}

	head, inline, hasColon := strings.Cut(dec, ":")
	key, ok := lookupKey(head)
	if !ok {
		return "", "", false
	}
	if hasColon {
		return key, strings.Trim(inline, "#>*` \t"), true
	}
	return key, "", true
}

// lookupKey сопоставляет текст заголовка каноническому ключу секции.
// Сначала точное совпадение, затем нечётное (опечатки моделей):
// до 1 правки для коротких заголовков, до 2 для длинных.
func lookupKey(head string) (string, bool) {
	norm := normalizeLabel(head)
	if norm == "" {
		return "", false
	}
	for _, entry := range sortedAliases {
		if norm == entry.alias {
			return entry.key, true
		}
	}
	maxDist := 0
	switch {
	case len([]rune(norm)) >= 5:
		maxDist = 2
	case len([]rune(norm)) >= 4:
		maxDist = 1
	default:
		return "", false
	}
	best, bestDist := "", maxDist+1
	for _, entry := range sortedAliases {
		d := levenshtein(norm, entry.alias, maxDist)
		if d < bestDist {
			best, bestDist = entry.key, d
		}
	}
	if bestDist <= maxDist {
		return best, true
	}
	return "", false
}

// parseDocument разбирает ответ модели на преамбулу и секции.
func parseDocument(raw string) document {
	doc := document{sections: map[string]*section{}}
	var current *section

	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if key, inline, ok := matchHeader(line); ok {
			current = &section{inline: inline}
			if _, seen := doc.sections[key]; !seen {
				doc.order = append(doc.order, key)
			}
			// Повторный заголовок дополняет существующую секцию.
			if prev, seen := doc.sections[key]; seen {
				current = prev
				if inline != "" {
					current.inline = joinNonEmpty(current.inline, inline)
				}
				continue
			}
			doc.sections[key] = current
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if current == nil {
			doc.preamble = append(doc.preamble, strings.TrimSpace(line))
		} else {
			current.body = append(current.body, strings.TrimSpace(line))
		}
	}
	return doc
}

// joinNonEmpty склеивает непустые строки через пробел.
func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return strings.Join(out, " ")
}

// listItemMarkers — маркеры элементов списка.
const listItemMarkers = "-*•–—"

// isListItem распознаёт строку-элемент списка: "- текст", "* текст", "1. текст".
func isListItem(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return false
	}
	if strings.IndexByte(listItemMarkers, s[0]) >= 0 {
		return true
	}
	if idx := strings.IndexAny(s, ".)"); idx > 0 && idx <= 3 && isDigits(s[:idx]) {
		return true
	}
	return false
}

// stripListMarker убирает маркер элемента списка.
func stripListMarker(line string) string {
	s := strings.TrimSpace(line)
	if strings.IndexByte(listItemMarkers, s[0]) >= 0 {
		return strings.TrimSpace(s[1:])
	}
	if idx := strings.IndexAny(s, ".)"); idx > 0 && idx <= 3 && isDigits(s[:idx]) {
		return strings.TrimSpace(s[idx+1:])
	}
	return s
}

// textValue возвращает скалярное значение секции: инлайн-текст плюс тело.
func (d *document) textValue(key string) string {
	s, ok := d.sections[key]
	if !ok {
		return ""
	}
	return joinNonEmpty(append([]string{s.inline}, s.body...)...)
}

// listValue возвращает список элементов секции. Строки с маркерами дают
// отдельные элементы; тело без маркеров считается одним элементом.
func (d *document) listValue(key string) []string {
	s, ok := d.sections[key]
	if !ok {
		return nil
	}
	var items []string
	if strings.TrimSpace(s.inline) != "" {
		items = append(items, strings.TrimSpace(s.inline))
	}
	var plain []string
	for _, line := range s.body {
		if isListItem(line) {
			items = append(items, stripListMarker(line))
			continue
		}
		plain = append(plain, line)
	}
	if len(plain) > 0 {
		items = append(items, joinNonEmpty(plain...))
	}
	return items
}

// firstLine возвращает первую содержательную строку списка строк.
func firstLine(lines []string) string {
	for _, l := range lines {
		if t := strings.TrimSpace(stripDecoration(l)); t != "" {
			return t
		}
	}
	return ""
}

// parseConfidence извлекает уверенность: первое число секции.
// Поддерживает «0,8», «80%»; при отсутствии — нейтральные 0.5.
func parseConfidence(doc *document) float64 {
	raw := doc.textValue("confidence")
	var number string
	for _, r := range raw {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' {
			number += string(r)
			continue
		}
		if number != "" {
			break
		}
	}
	if number == "" {
		return defaultConfidence
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(number, ",", "."), 64)
	if err != nil || math.IsNaN(v) {
		return defaultConfidence
	}
	if v > 1 && v <= 100 {
		v /= 100 // модель ответила в процентах
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// defaultConfidence — уверенность, когда модель её не указала.
const defaultConfidence = 0.5

// fallbackDecision — дополнительная обработка, когда секция РЕШЕНИЕ не найдена:
// берём первую содержательную строку ответа как решение.
func fallbackDecision(doc *document) string {
	if v := firstLine(doc.preamble); v != "" {
		return v
	}
	// Последний шанс: первая строка любой секции-списка.
	for _, key := range doc.order {
		if items := doc.listValue(key); len(items) > 0 {
			return items[0]
		}
	}
	return ""
}

// ParseProposal разбирает текстовый ответ LLM в доменную структуру Proposal.
// ID и ParticipantID проставляет движок — модель их не возвращает.
func ParseProposal(raw string) (domain.Proposal, error) {
	doc := parseDocument(raw)
	decision := doc.textValue("decision")
	if decision == "" {
		decision = fallbackDecision(&doc)
	}
	if decision == "" {
		return domain.Proposal{}, fmt.Errorf("debate: в ответе нет решения")
	}
	return domain.Proposal{
		Decision:    decision,
		Arguments:   doc.listValue("arguments"),
		Assumptions: doc.listValue("assumptions"),
		Risks:       doc.listValue("risks"),
		Confidence:  parseConfidence(&doc),
	}, nil
}

// ParseCritique разбирает текстовый ответ LLM в доменную структуру Critique.
func ParseCritique(raw string) (domain.Critique, error) {
	doc := parseDocument(raw)
	c := domain.Critique{
		ValidPoints:        doc.listValue("valid_points"),
		Errors:             doc.listValue("errors"),
		MissingInformation: doc.listValue("missing_information"),
		Risks:              doc.listValue("risks"),
		CounterArguments:   doc.listValue("counter_arguments"),
		ProposedChanges:    doc.listValue("proposed_changes"),
	}
	if !c.HasContent() {
		return domain.Critique{}, fmt.Errorf("debate: критика пуста")
	}
	return c, nil
}

// ParseConsensusVerdict разбирает текстовый вердикт арбитра.
// Если тип согласия не распознан — честный INSUFFICIENT_DATA,
// а не искусственный выбор победителя.
func ParseConsensusVerdict(raw string) (ConsensusVerdict, error) {
	doc := parseDocument(raw)

	agreement := AgreementInsufficientData
	haystack := normalizeLabel(doc.textValue("agreement"))
	if haystack == "" {
		haystack = normalizeLabel(raw)
	}
	switch {
	case containsAny(haystack,
		normalizeLabel("insufficient_data"), normalizeLabel("insufficient data"),
		normalizeLabel("недостаточно данных"), normalizeLabel("недостаточно информации")):
		agreement = AgreementInsufficientData
	case containsAny(haystack,
		normalizeLabel("disagreement"), normalizeLabel("несогласие"), normalizeLabel("разногласие")):
		agreement = AgreementDisagreement
	case containsAny(haystack,
		normalizeLabel("consensus"), normalizeLabel("консенсус")):
		agreement = AgreementConsensus
	}

	decision := doc.textValue("decision")
	if decision == "" {
		decision = fallbackDecision(&doc)
	}

	confidence := parseConfidence(&doc)
	if math.IsNaN(confidence) {
		confidence = defaultConfidence
	}

	return ConsensusVerdict{
		Agreement:    agreement,
		Decision:     decision,
		Requirements: doc.listValue("requirements"),
		Arguments:    doc.listValue("arguments"),
		Risks:        doc.listValue("risks"),
		Confidence:   confidence,
		Reasoning:    doc.textValue("reasoning"),
	}, nil
}

// containsAny сообщает, содержит ли строка хотя бы одну подстроку.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Сборка текстовых ответов (для mock-серверов и тестов).

// ProposalToText сериализует предложение в текстовый протокол.
func ProposalToText(p domain.Proposal) string {
	var b strings.Builder
	b.WriteString("РЕШЕНИЕ: " + p.Decision + "\n")
	writeList := func(header string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString(header + ":\n")
		for _, item := range items {
			b.WriteString("- " + item + "\n")
		}
	}
	writeList("АРГУМЕНТЫ", p.Arguments)
	writeList("ДОПУЩЕНИЯ", p.Assumptions)
	writeList("РИСКИ", p.Risks)
	fmt.Fprintf(&b, "УВЕРЕННОСТЬ: %.2f\n", p.Confidence)
	return b.String()
}

// CritiqueToText сериализует критику в текстовый протокол.
func CritiqueToText(c domain.Critique) string {
	var b strings.Builder
	writeList := func(header string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString(header + ":\n")
		for _, item := range items {
			b.WriteString("- " + item + "\n")
		}
	}
	writeList("ВЕРНЫЕ УТВЕРЖДЕНИЯ", c.ValidPoints)
	writeList("ОШИБКИ", c.Errors)
	writeList("НЕ ХВАТАЕТ ИНФОРМАЦИИ", c.MissingInformation)
	writeList("РИСКИ", c.Risks)
	writeList("КОНТРАРГУМЕНТЫ", c.CounterArguments)
	writeList("ПРЕДЛАГАЕМЫЕ ИЗМЕНЕНИЯ", c.ProposedChanges)
	return b.String()
}

// VerdictToText сериализует вердикт арбитра в текстовый протокол.
func VerdictToText(v ConsensusVerdict) string {
	var b strings.Builder
	b.WriteString("СОГЛАСИЕ: " + string(v.Agreement) + "\n")
	if v.Decision != "" {
		b.WriteString("РЕШЕНИЕ: " + v.Decision + "\n")
	}
	writeList := func(header string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString(header + ":\n")
		for _, item := range items {
			b.WriteString("- " + item + "\n")
		}
	}
	writeList("ТРЕБОВАНИЯ", v.Requirements)
	writeList("АРГУМЕНТЫ", v.Arguments)
	writeList("РИСКИ", v.Risks)
	fmt.Fprintf(&b, "УВЕРЕННОСТЬ: %.2f\n", v.Confidence)
	if v.Reasoning != "" {
		b.WriteString("ОБОСНОВАНИЕ: " + v.Reasoning + "\n")
	}
	return b.String()
}

// ParseSimilarity разбирает ответ арбитра с коэффициентом смыслового
// совпадения решений. Ожидается одно число от 0 до 1; допустимы
// запятая как разделитель и процентная запись («90%»).
func ParseSimilarity(content string) (float64, error) {
	var token []rune
	for _, r := range []rune(strings.TrimSpace(content)) {
		isTokenRune := (r >= '0' && r <= '9') || r == '.' || r == ',' || r == '%'
		if isTokenRune {
			token = append(token, r)
			continue
		}
		if len(token) > 0 {
			break
		}
	}
	if len(token) == 0 {
		return 0, fmt.Errorf("debate: в ответе арбитра нет числа: %q", content)
	}

	t := string(token)
	percent := strings.HasSuffix(t, "%")
	t = strings.TrimSuffix(t, "%")
	t = strings.ReplaceAll(t, ",", ".")

	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, fmt.Errorf("debate: не удалось разобрать коэффициент из %q: %w", content, err)
	}
	if percent {
		v /= 100
	}
	if v < 0 || v > 1 {
		return 0, fmt.Errorf("debate: коэффициент совпадения %v вне диапазона [0,1]", v)
	}
	return v, nil
}
