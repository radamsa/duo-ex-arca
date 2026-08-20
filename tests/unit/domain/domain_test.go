// Тесты domain-модели: Task, Proposal, Critique, Decision, Debate.
package domain_test

import (
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/domain"
)

// --- TaskMode ---

func TestTaskModeValid(t *testing.T) {
	valid := []domain.TaskMode{
		domain.FAST,
		domain.NORMAL,
		domain.DELIBERATE,
		domain.CRITICAL,
	}
	for _, m := range valid {
		if !m.Valid() {
			t.Errorf("%q должен быть валидным режимом", m)
		}
	}
}

func TestTaskModeInvalid(t *testing.T) {
	invalid := []domain.TaskMode{
		"",
		"fast",
		"SUPER_FAST",
		" NORMAL",
	}
	for _, m := range invalid {
		if m.Valid() {
			t.Errorf("%q не должен быть валидным режимом", m)
		}
	}
}

// --- Task ---

func TestNewTaskValid(t *testing.T) {
	task, err := domain.NewTask("t-1", "Какую БД использовать?", "Нужен выбор СУБД", []string{"только Go"}, domain.NORMAL)
	if err != nil {
		t.Fatalf("NewTask вернул ошибку: %v", err)
	}
	if task.ID != "t-1" || task.Mode != domain.NORMAL {
		t.Fatalf("поля Task заполнены неверно: %+v", task)
	}
	if len(task.Constraints) != 1 || task.Constraints[0] != "только Go" {
		t.Fatalf("Constraints заполнены неверно: %+v", task.Constraints)
	}
}

func TestNewTaskInvalid(t *testing.T) {
	cases := []struct {
		name  string
		id    string
		title string
		mode  domain.TaskMode
	}{
		{"пустой ID", "", "Заголовок", domain.NORMAL},
		{"пустой заголовок", "t-1", "", domain.NORMAL},
		{"невалидный режим", "t-1", "Заголовок", "BAD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.NewTask(tc.id, tc.title, "", nil, tc.mode); err == nil {
				t.Error("ожидалась ошибка валидации")
			}
		})
	}
}

// --- Proposal ---

func TestNewProposalValid(t *testing.T) {
	p, err := domain.NewProposal("p-1", "participant-a", "Использовать SQLite", []string{"ноль настроек"}, []string{"лимит записи"}, []string{"потеря файла"}, 0.9)
	if err != nil {
		t.Fatalf("NewProposal вернул ошибку: %v", err)
	}
	if p.Decision != "Использовать SQLite" || p.Confidence != 0.9 {
		t.Fatalf("поля Proposal заполнены неверно: %+v", p)
	}
}

func TestNewProposalInvalid(t *testing.T) {
	cases := []struct {
		name        string
		participant string
		decision    string
		confidence  float64
	}{
		{"пустой участник", "", "Решение", 0.5},
		{"пустое решение", "participant-a", "", 0.5},
		{"отрицательная уверенность", "participant-a", "Решение", -0.1},
		{"уверенность больше единицы", "participant-a", "Решение", 1.1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.NewProposal("p-1", tc.participant, tc.decision, nil, nil, nil, tc.confidence); err == nil {
				t.Error("ожидалась ошибка валидации")
			}
		})
	}
}

// --- Critique ---

func TestCritiqueHasContent(t *testing.T) {
	empty := domain.Critique{}
	if empty.HasContent() {
		t.Error("пустая критика не должна иметь содержимого")
	}

	withErrors := domain.Critique{Errors: []string{"неверный вывод"}}
	if !withErrors.HasContent() {
		t.Error("критика с ошибками должна иметь содержимое")
	}

	withMissing := domain.Critique{MissingInformation: []string{"нет данных о нагрузке"}}
	if !withMissing.HasContent() {
		t.Error("критика с недостающей информацией должна иметь содержимое")
	}
}

func TestCritiqueValidate(t *testing.T) {
	if err := (domain.Critique{}).Validate(); err == nil {
		t.Error("полностью пустая критика должна считаться невалидной")
	}
	if err := (domain.Critique{ValidPoints: []string{"аргумент верен"}}).Validate(); err != nil {
		t.Errorf("критика с содержимым должна быть валидной: %v", err)
	}
}

// --- Decision ---

func TestDecisionStatusValid(t *testing.T) {
	valid := []domain.DecisionStatus{
		domain.Consensus,
		domain.Disagreement,
		domain.InsufficientData,
		domain.RequireUserInput,
		domain.Failed,
	}
	for _, s := range valid {
		if !s.Valid() {
			t.Errorf("%q должен быть валидным статусом", s)
		}
	}
	if (domain.DecisionStatus("UNKNOWN")).Valid() {
		t.Error("неизвестный статус не должен быть валидным")
	}
}

func TestNewDecisionValid(t *testing.T) {
	d, err := domain.NewDecision(domain.Consensus, "Использовать SQLite", 0.9)
	if err != nil {
		t.Fatalf("NewDecision вернул ошибку: %v", err)
	}
	if d.Status != domain.Consensus || d.Decision != "Использовать SQLite" || d.Confidence != 0.9 {
		t.Fatalf("поля Decision заполнены неверно: %+v", d)
	}
}

func TestNewDecisionInvalid(t *testing.T) {
	cases := []struct {
		name       string
		status     domain.DecisionStatus
		decision   string
		confidence float64
	}{
		{"невалидный статус", "BAD", "Решение", 0.5},
		{"пустое решение при статусе Consensus", domain.Consensus, "", 0.5},
		{"отрицательная уверенность", domain.Consensus, "Решение", -0.5},
		{"уверенность больше единицы", domain.Consensus, "Решение", 1.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.NewDecision(tc.status, tc.decision, tc.confidence); err == nil {
				t.Error("ожидалась ошибка валидации")
			}
		})
	}
}

func TestNewDecisionFailedAllowsEmpty(t *testing.T) {
	if _, err := domain.NewDecision(domain.Failed, "", 0); err != nil {
		t.Errorf("решение со статусом FAILED может не иметь решения: %v", err)
	}
}

// TestNewDecisionNonConsensusAllowsEmpty — штатные не-консенсусные
// результаты (Disagreement, InsufficientData, RequireUserInput) могут
// не содержать текста решения.
func TestNewDecisionNonConsensusAllowsEmpty(t *testing.T) {
	for _, status := range []domain.DecisionStatus{
		domain.Disagreement,
		domain.InsufficientData,
		domain.RequireUserInput,
	} {
		if _, err := domain.NewDecision(status, "", 0.5); err != nil {
			t.Errorf("статус %s должен допускать пустое решение: %v", status, err)
		}
	}
}

// --- DebateRound ---

func TestNewDebateRound(t *testing.T) {
	if _, err := domain.NewDebateRound(0); err == nil {
		t.Error("номер раунда 0 должен быть невалидным")
	}
	r, err := domain.NewDebateRound(1)
	if err != nil {
		t.Fatalf("NewDebateRound вернул ошибку: %v", err)
	}
	if r.Number != 1 {
		t.Fatalf("номер раунда: %d", r.Number)
	}
}

func TestDebateRoundIsComplete(t *testing.T) {
	r, _ := domain.NewDebateRound(1)
	if r.IsComplete() {
		t.Error("пустой раунд не должен быть полным")
	}

	mustProposal := func() *domain.Proposal {
		p, err := domain.NewProposal("p", "participant-a", "Решение", nil, nil, nil, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		return &p
	}
	mustCritique := func() *domain.Critique {
		c := domain.Critique{Errors: []string{"ошибка"}}
		return &c
	}

	r.ProposalA = mustProposal()
	r.ProposalB = mustProposal()
	r.CritiqueA = mustCritique()
	r.CritiqueB = mustCritique()
	r.RevisionA = mustProposal()
	r.RevisionB = mustProposal()

	if !r.IsComplete() {
		t.Error("полностью заполненный раунд должен считаться полным")
	}

	r.CritiqueB = nil
	if r.IsComplete() {
		t.Error("раунд без одной критики не должен быть полным")
	}
}

// --- Debate ---

func TestNewDebateValid(t *testing.T) {
	task, _ := domain.NewTask("t-1", "Заголовок", "", nil, domain.NORMAL)
	d, err := domain.NewDebate("d-1", task)
	if err != nil {
		t.Fatalf("NewDebate вернул ошибку: %v", err)
	}
	if d.ID != "d-1" || d.Task.ID != "t-1" {
		t.Fatalf("поля Debate заполнены неверно: %+v", d)
	}
	if d.RoundsCount() != 0 {
		t.Fatalf("новый дебат не должен содержать раундов")
	}
	if d.HasDecision() {
		t.Fatal("новый дебат не должен содержать решения")
	}
}

func TestNewDebateInvalidTask(t *testing.T) {
	// Неконструируемая задача — создадим через zero value и проверим ошибку.
	if _, err := domain.NewDebate("d-1", domain.Task{}); err == nil {
		t.Error("дебат с невалидной задачей должен вернуть ошибку")
	}
}

func TestDebateAddRound(t *testing.T) {
	task, _ := domain.NewTask("t-1", "Заголовок", "", nil, domain.NORMAL)
	d, _ := domain.NewDebate("d-1", task)

	r1, _ := domain.NewDebateRound(1)
	if err := d.AddRound(r1); err != nil {
		t.Fatalf("AddRound вернул ошибку: %v", err)
	}

	r2, _ := domain.NewDebateRound(2)
	if err := d.AddRound(r2); err != nil {
		t.Fatalf("AddRound вернул ошибку: %v", err)
	}

	if d.RoundsCount() != 2 {
		t.Fatalf("ожидалось 2 раунда, получено %d", d.RoundsCount())
	}

	// Повторный номер раунда — ошибка.
	if err := d.AddRound(r1); err == nil {
		t.Error("добавление раунда с повторным номером должно вернуть ошибку")
	}
}

func TestDebateSetDecision(t *testing.T) {
	task, _ := domain.NewTask("t-1", "Заголовок", "", nil, domain.NORMAL)
	d, _ := domain.NewDebate("d-1", task)

	dec, _ := domain.NewDecision(domain.Consensus, "Решение", 0.9)
	d.SetDecision(dec)

	if !d.HasDecision() {
		t.Fatal("решение должно быть установлено")
	}
	if d.Decision.Status != domain.Consensus {
		t.Fatalf("неверный статус решения: %s", d.Decision.Status)
	}
}