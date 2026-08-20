package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/storage"
)

// DebateRepository — SQLite-реализация storage.DebateRepository.
type DebateRepository struct {
	db *DB
}

// NewDebateRepository создаёт репозиторий дебатов.
func NewDebateRepository(db *DB) *DebateRepository {
	return &DebateRepository{db: db}
}

// roundRow — строка таблицы debate_rounds.
type roundRow struct {
	roundNumber int
	proposalA   string
	proposalB   string
	critiqueA   string
	critiqueB   string
	revisionA   string
	revisionB   string
}

// Save сохраняет дебат с раундами и решением в одной транзакции.
func (r *DebateRepository) Save(ctx context.Context, debate domain.Debate) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: начало транзакции дебата: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO debates (id, task_id) VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET task_id = excluded.task_id`,
		debate.ID, debate.Task.ID,
	); err != nil {
		return fmt.Errorf("sqlite: сохранение дебата: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM debate_rounds WHERE debate_id = ?`, debate.ID); err != nil {
		return fmt.Errorf("sqlite: очистка раундов: %w", err)
	}
	for _, round := range debate.Rounds {
		if err := insertRound(ctx, tx, debate.ID, round); err != nil {
			return err
		}
	}

	if err := upsertDecision(ctx, tx, debate); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: коммит дебата: %w", err)
	}
	return nil
}

// insertRound записывает один раунд.
func insertRound(ctx context.Context, tx *sql.Tx, debateID string, round domain.DebateRound) error {
	proposalA, err := marshalProp(round.ProposalA)
	if err != nil {
		return err
	}
	proposalB, err := marshalProp(round.ProposalB)
	if err != nil {
		return err
	}
	critiqueA, err := marshalCritique(round.CritiqueA)
	if err != nil {
		return err
	}
	critiqueB, err := marshalCritique(round.CritiqueB)
	if err != nil {
		return err
	}
	revisionA, err := marshalProp(round.RevisionA)
	if err != nil {
		return err
	}
	revisionB, err := marshalProp(round.RevisionB)
	if err != nil {
		return err
	}

	const query = `
INSERT INTO debate_rounds
	(debate_id, round_number, proposal_a, proposal_b, critique_a, critique_b, revision_a, revision_b)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, query,
		debateID, round.Number, proposalA, proposalB, critiqueA, critiqueB, revisionA, revisionB,
	); err != nil {
		return fmt.Errorf("sqlite: сохранение раунда %d: %w", round.Number, err)
	}
	return nil
}

// upsertDecision сохраняет решение дебата.
func upsertDecision(ctx context.Context, tx *sql.Tx, debate domain.Debate) error {
	if debate.Decision == nil {
		return nil
	}
	dec := debate.Decision

	suppArgs, err := json.Marshal(dec.SupportingArguments)
	if err != nil {
		return fmt.Errorf("sqlite: сериализация аргументов решения: %w", err)
	}
	rejectArgs, err := json.Marshal(dec.RejectedArguments)
	if err != nil {
		return fmt.Errorf("sqlite: сериализация отклонённых аргументов: %w", err)
	}
	risks, err := json.Marshal(dec.Risks)
	if err != nil {
		return fmt.Errorf("sqlite: сериализация рисков решения: %w", err)
	}
	unresolved, err := json.Marshal(dec.UnresolvedIssues)
	if err != nil {
		return fmt.Errorf("sqlite: сериализация нерешённых вопросов: %w", err)
	}

	const query = `
INSERT INTO decisions
	(debate_id, status, decision, confidence, supporting_arguments, rejected_arguments, risks, unresolved_issues)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(debate_id) DO UPDATE SET
	status = excluded.status,
	decision = excluded.decision,
	confidence = excluded.confidence,
	supporting_arguments = excluded.supporting_arguments,
	rejected_arguments = excluded.rejected_arguments,
	risks = excluded.risks,
	unresolved_issues = excluded.unresolved_issues`
	if _, err := tx.ExecContext(ctx, query,
		debate.ID, string(dec.Status), dec.Decision, dec.Confidence,
		string(suppArgs), string(rejectArgs), string(risks), string(unresolved),
	); err != nil {
		return fmt.Errorf("sqlite: сохранение решения: %w", err)
	}
	return nil
}

// Get полностью восстанавливает дебат с раундами и решением.
func (r *DebateRepository) Get(ctx context.Context, id string) (domain.Debate, error) {
	var debateID, taskID string
	err := r.db.QueryRowContext(ctx, `SELECT id, task_id FROM debates WHERE id = ?`, id).Scan(&debateID, &taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Debate{}, storage.ErrNotFound
	}
	if err != nil {
		return domain.Debate{}, fmt.Errorf("sqlite: чтение дебата: %w", err)
	}

	task, err := r.loadTask(ctx, taskID)
	if err != nil {
		return domain.Debate{}, err
	}

	debate, err := domain.NewDebate(debateID, task)
	if err != nil {
		return domain.Debate{}, err
	}

	rounds, err := r.loadRounds(ctx, id)
	if err != nil {
		return domain.Debate{}, err
	}
	for _, round := range rounds {
		if err := debate.AddRound(round); err != nil {
			return domain.Debate{}, fmt.Errorf("sqlite: восстановление раундов: %w", err)
		}
	}

	decision, err := r.loadDecision(ctx, id)
	if err != nil {
		return domain.Debate{}, err
	}
	if decision != nil {
		debate.SetDecision(*decision)
	}
	return debate, nil
}

// loadTask читает задачу дебата.
func (r *DebateRepository) loadTask(ctx context.Context, taskID string) (domain.Task, error) {
	tasks := NewTaskRepository(r.db)
	task, err := tasks.Get(ctx, taskID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("sqlite: задача дебата: %w", err)
	}
	return task, nil
}

// loadRounds читает и собирает раунды дебата.
func (r *DebateRepository) loadRounds(ctx context.Context, debateID string) ([]domain.DebateRound, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT round_number, proposal_a, proposal_b, critique_a, critique_b, revision_a, revision_b
FROM debate_rounds WHERE debate_id = ? ORDER BY round_number`, debateID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: чтение раундов: %w", err)
	}
	defer rows.Close()

	var rounds []domain.DebateRound
	for rows.Next() {
		var row roundRow
		if err := rows.Scan(&row.roundNumber, &row.proposalA, &row.proposalB,
			&row.critiqueA, &row.critiqueB, &row.revisionA, &row.revisionB); err != nil {
			return nil, fmt.Errorf("sqlite: чтение строки раунда: %w", err)
		}

		round, err := domain.NewDebateRound(row.roundNumber)
		if err != nil {
			return nil, err
		}
		if round.ProposalA, err = unmarshalProp(row.proposalA); err != nil {
			return nil, err
		}
		if round.ProposalB, err = unmarshalProp(row.proposalB); err != nil {
			return nil, err
		}
		if round.CritiqueA, err = unmarshalCritique(row.critiqueA); err != nil {
			return nil, err
		}
		if round.CritiqueB, err = unmarshalCritique(row.critiqueB); err != nil {
			return nil, err
		}
		if round.RevisionA, err = unmarshalProp(row.revisionA); err != nil {
			return nil, err
		}
		if round.RevisionB, err = unmarshalProp(row.revisionB); err != nil {
			return nil, err
		}
		rounds = append(rounds, round)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: чтение раундов: %w", err)
	}
	return rounds, nil
}

// loadDecision читает решение дебата (nil, если решения нет).
func (r *DebateRepository) loadDecision(ctx context.Context, debateID string) (*domain.Decision, error) {
	var dec domain.Decision
	var status string
	var suppArgs, rejectArgs, risks, unresolved string

	err := r.db.QueryRowContext(ctx, `
SELECT status, decision, confidence, supporting_arguments, rejected_arguments, risks, unresolved_issues
FROM decisions WHERE debate_id = ?`, debateID).Scan(
		&status, &dec.Decision, &dec.Confidence, &suppArgs, &rejectArgs, &risks, &unresolved,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: чтение решения: %w", err)
	}

	dec.Status = domain.DecisionStatus(status)

	jsonFields := []struct {
		raw  string
		dst  *[]string
		name string
	}{
		{suppArgs, &dec.SupportingArguments, "аргументов решения"},
		{rejectArgs, &dec.RejectedArguments, "отклонённых аргументов"},
		{risks, &dec.Risks, "рисков решения"},
		{unresolved, &dec.UnresolvedIssues, "нерешённых вопросов"},
	}
	for _, field := range jsonFields {
		if err := json.Unmarshal([]byte(field.raw), field.dst); err != nil {
			return nil, fmt.Errorf("sqlite: разбор %s: %w", field.name, err)
		}
	}
	return &dec, nil
}

// ListByTask возвращает дебаты задачи.
func (r *DebateRepository) ListByTask(ctx context.Context, taskID string) ([]domain.Debate, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM debates WHERE task_id = ? ORDER BY id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: список дебатов: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sqlite: чтение id дебата: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: список дебатов: %w", err)
	}

	var debates []domain.Debate
	for _, id := range ids {
		d, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		debates = append(debates, d)
	}
	return debates, nil
}

// marshalProp сериализует предложение (nil -> "{}").
func marshalProp(p *domain.Proposal) (string, error) {
	if p == nil {
		return "{}", nil
	}
	data, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("sqlite: сериализация предложения: %w", err)
	}
	return string(data), nil
}

// unmarshalProp разбирает предложение ("{}" -> nil).
func unmarshalProp(raw string) (*domain.Proposal, error) {
	if raw == "{}" {
		return nil, nil
	}
	var p domain.Proposal
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("sqlite: разбор предложения: %w", err)
	}
	return &p, nil
}

// marshalCritique сериализует критику (nil -> "{}").
func marshalCritique(c *domain.Critique) (string, error) {
	if c == nil {
		return "{}", nil
	}
	data, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("sqlite: сериализация критики: %w", err)
	}
	return string(data), nil
}

// unmarshalCritique разбирает критику ("{}" -> nil).
func unmarshalCritique(raw string) (*domain.Critique, error) {
	if raw == "{}" {
		return nil, nil
	}
	var c domain.Critique
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, fmt.Errorf("sqlite: разбор критики: %w", err)
	}
	return &c, nil
}