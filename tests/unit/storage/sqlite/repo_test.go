// Тесты SQLite-репозиториев (TASK-080..086, TASK-091).
package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/storage"
	"github.com/radamsa/duo-ex-arca/internal/storage/sqlite"
	"github.com/radamsa/duo-ex-arca/internal/trace"
)

// openDB открывает базу во временном файле.
func openDB(t *testing.T) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "arca-test.db"))
	if err != nil {
		t.Fatalf("Open вернул ошибку: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustTask(t *testing.T, id string) domain.Task {
	t.Helper()
	task, err := domain.NewTask(id, "Какую БД выбрать?", "Нужна СУБД", []string{"только Go"}, domain.NORMAL)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

// TestOpenCreatesSchema — при открытии схема создаётся автоматически (TASK-080).
func TestOpenCreatesSchema(t *testing.T) {
	db := openDB(t)

	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&count); err != nil {
		t.Fatalf("запрос к sqlite_master не удался: %v", err)
	}
	wantTables := []string{"tasks", "debates", "debate_rounds", "decisions", "traces"}
	for _, table := range wantTables {
		var exists int
		db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&exists)
		if exists != 1 {
			t.Errorf("таблица %s не создана", table)
		}
	}
}

// TestOpenInvalidPath — невалидный путь — ошибка.
func TestOpenInvalidPath(t *testing.T) {
	if _, err := sqlite.Open("/нет/такого/каталога/arca.db"); err == nil {
		t.Fatal("ожидалась ошибка открытия")
	}
}

// TestTaskRepositoryRoundTrip — сохранение и чтение задачи (TASK-081).
func TestTaskRepositoryRoundTrip(t *testing.T) {
	db := openDB(t)
	repo := sqlite.NewTaskRepository(db)

	task := mustTask(t, "task-1")
	if err := repo.Save(context.Background(), task); err != nil {
		t.Fatalf("Save вернул ошибку: %v", err)
	}

	got, err := repo.Get(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Get вернул ошибку: %v", err)
	}
	if got.ID != task.ID || got.Title != task.Title || got.Mode != task.Mode {
		t.Fatalf("прочитанная задача не совпадает: %+v", got)
	}
	if len(got.Constraints) != 1 || got.Constraints[0] != "только Go" {
		t.Fatalf("ограничения не сохранились: %+v", got.Constraints)
	}
}

// TestTaskRepositoryNotFound — отсутствующая задача — ErrNotFound.
func TestTaskRepositoryNotFound(t *testing.T) {
	db := openDB(t)
	repo := sqlite.NewTaskRepository(db)

	if _, err := repo.Get(context.Background(), "нет-такой"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("ожидался ErrNotFound, получено: %v", err)
	}
}

// makeDebate собирает дебат с одним полным раундом и решением.
func makeDebate(t *testing.T, id, taskID string) domain.Debate {
	t.Helper()
	task := mustTask(t, taskID)
	d, err := domain.NewDebate(id, task)
	if err != nil {
		t.Fatal(err)
	}

	round, err := domain.NewDebateRound(1)
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Proposal{Decision: "SQLite", Confidence: 0.9}
	c := domain.Critique{Errors: []string{"нет нагрузки"}}
	r := domain.Proposal{Decision: "SQLite с WAL", Confidence: 0.9}
	round.ProposalA = &p
	round.ProposalB = &p
	round.CritiqueA = &c
	round.CritiqueB = &c
	round.RevisionA = &r
	round.RevisionB = &r
	if err := d.AddRound(round); err != nil {
		t.Fatal(err)
	}

	decision, err := domain.NewDecision(domain.Consensus, "SQLite с WAL", 0.9)
	if err != nil {
		t.Fatal(err)
	}
	decision.SupportingArguments = []string{"низкая стоимость"}
	d.SetDecision(decision)
	return d
}

// TestDebateRepositoryRoundTrip — сохранение и полное восстановление
// дебата с раундами и решением (TASK-082..084).
func TestDebateRepositoryRoundTrip(t *testing.T) {
	db := openDB(t)
	repo := sqlite.NewDebateRepository(db)
	tasks := sqlite.NewTaskRepository(db)

	want := makeDebate(t, "debate-1", "task-1")
	if err := tasks.Save(context.Background(), want.Task); err != nil {
		t.Fatalf("Save задачи: %v", err)
	}
	if err := repo.Save(context.Background(), want); err != nil {
		t.Fatalf("Save дебата: %v", err)
	}

	got, err := repo.Get(context.Background(), "debate-1")
	if err != nil {
		t.Fatalf("Get вернул ошибку: %v", err)
	}

	if got.ID != want.ID || got.Task.ID != want.Task.ID {
		t.Fatalf("дебат не совпадает: %+v", got)
	}
	if got.RoundsCount() != 1 {
		t.Fatalf("раунды не восстановлены: %d", got.RoundsCount())
	}
	round := got.Rounds[0]
	if !round.IsComplete() {
		t.Fatal("раунд не полон после восстановления")
	}
	if round.ProposalA.Decision != "SQLite" || round.RevisionB.Decision != "SQLite с WAL" {
		t.Fatalf("содержимое раунда не совпадает: %+v", round)
	}
	if !got.HasDecision() {
		t.Fatal("решение не восстановлено")
	}
	if got.Decision.Status != domain.Consensus || got.Decision.Decision != "SQLite с WAL" {
		t.Fatalf("решение не совпадает: %+v", got.Decision)
	}
	if len(got.Decision.SupportingArguments) != 1 {
		t.Fatalf("аргументы решения не восстановлены: %+v", got.Decision.SupportingArguments)
	}
}

// TestDebateRepositoryListByTask — список дебатов по задаче (TASK-082).
func TestDebateRepositoryListByTask(t *testing.T) {
	db := openDB(t)
	repo := sqlite.NewDebateRepository(db)
	tasks := sqlite.NewTaskRepository(db)

	task := mustTask(t, "task-1")
	if err := tasks.Save(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	d1 := makeDebate(t, "debate-1", "task-1")
	d2 := makeDebate(t, "debate-2", "task-1")
	for _, d := range []domain.Debate{d1, d2} {
		if err := repo.Save(context.Background(), d); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	list, err := repo.ListByTask(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("ListByTask вернул ошибку: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ожидалось 2 дебата, получено %d", len(list))
	}
}

// TestTraceRepositoryAppendAndList — события сохраняются в порядке
// добавления (TASK-085, TASK-091).
func TestTraceRepositoryAppendAndList(t *testing.T) {
	db := openDB(t)
	repo := sqlite.NewTraceRepository(db)

	makeEvent := func(typ trace.EventType, ts time.Time) trace.Event {
		return trace.Event{
			TraceID:   "trace-1",
			TaskID:    "task-1",
			Timestamp: ts,
			Type:      typ,
			Duration:  15 * time.Millisecond,
			Metadata:  map[string]string{"k": "v"},
		}
	}

	base := time.Now().UTC()
	events := []trace.Event{
		makeEvent(trace.TaskCreated, base),
		makeEvent(trace.ContextBuilt, base.Add(time.Second)),
		makeEvent(trace.DecisionCreated, base.Add(2*time.Second)),
	}
	for _, ev := range events {
		if err := repo.Append(context.Background(), ev); err != nil {
			t.Fatalf("Append вернул ошибку: %v", err)
		}
	}

	got, err := repo.ListByTask(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("ListByTask вернул ошибку: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ожидалось 3 события, получено %d", len(got))
	}
	for i, ev := range got {
		if ev.Type != events[i].Type {
			t.Errorf("событие %d: тип %s, ожидался %s", i, ev.Type, events[i].Type)
		}
		if ev.TraceID != "trace-1" || ev.Metadata["k"] != "v" {
			t.Errorf("событие %d: поля не восстановлены: %+v", i, ev)
		}
		if ev.Timestamp.IsZero() {
			t.Errorf("событие %d: timestamp не восстановлен", i)
		}
	}
}

// TestTraceRepositoryNotFound — нет событий — пустой список, не ошибка.
func TestTraceRepositoryNotFound(t *testing.T) {
	db := openDB(t)
	repo := sqlite.NewTraceRepository(db)

	got, err := repo.ListByTask(context.Background(), "без-событий")
	if err != nil {
		t.Fatalf("ListByTask вернул ошибку: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ожидался пустой список, получено %d", len(got))
	}
}

// TestSQLiteImplementsInterfaces — компиляционная проверка контрактов
// repository interfaces (TASK-086: domain не знает SQL).
func TestSQLiteImplementsInterfaces(t *testing.T) {
	db := openDB(t)

	var _ storage.TaskRepository = sqlite.NewTaskRepository(db)
	var _ storage.DebateRepository = sqlite.NewDebateRepository(db)
	var _ storage.TraceRepository = sqlite.NewTraceRepository(db)
}