// Пакет agent — Agent Runner: pipeline Task -> Context -> Debate -> Decision.
//
// Для режимов NORMAL/DELIBERATE/CRITICAL используется Debate Core.
// Режим FAST обходит движок и опрашивает одного участника
// (docs/plan-mvp.md, TASK-070..071).
package agent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	ctxb "github.com/radamsa/duo-ex-arca/internal/context"
	"github.com/radamsa/duo-ex-arca/internal/debate"
	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/llm"
	"github.com/radamsa/duo-ex-arca/internal/trace"
)

// Runner выполняет полный pipeline агента.
type Runner struct {
	engine          *debate.Engine
	fastParticipant *debate.Participant
	contextBuilder  *ctxb.Builder
	trace           trace.Recorder
	notify          debate.NotifyFunc
}

// RunnerConfig — конфигурация runner'а.
type RunnerConfig struct {
	// Engine — Debate Core для режимов NORMAL/DELIBERATE/CRITICAL.
	Engine *debate.Engine

	// FastParticipant — единственный участник для режима FAST.
	FastParticipant *debate.Participant

	ContextBuilder *ctxb.Builder

	// Trace — рекордер событий трассировки (опционально).
	Trace trace.Recorder

	// Notify — колбэк активности для отображения прогресса
	// (опционально; при nil не вызывается).
	Notify debate.NotifyFunc
}

// NewRunner создаёт runner и проверяет конфигурацию.
func NewRunner(cfg RunnerConfig) (*Runner, error) {
	if cfg.Engine == nil {
		return nil, fmt.Errorf("agent: не задан Debate Engine")
	}
	if cfg.FastParticipant == nil {
		return nil, fmt.Errorf("agent: не задан участник для режима FAST")
	}
	if cfg.ContextBuilder == nil {
		return nil, fmt.Errorf("agent: не задан ContextBuilder")
	}
	if cfg.Trace == nil {
		cfg.Trace = trace.Noop{}
	}
	return &Runner{
		engine:          cfg.Engine,
		fastParticipant: cfg.FastParticipant,
		contextBuilder:  cfg.ContextBuilder,
		trace:           cfg.Trace,
		notify:          cfg.Notify,
	}, nil
}

// Run выполняет задачу и возвращает решение и дебат.
// Для режима FAST дебата нет (debate == nil).
// При любой ошибке решения возвращается Decision со статусом FAILED и ошибка.
func (r *Runner) Run(ctx context.Context, task domain.Task) (domain.Decision, *domain.Debate, error) {
	runID := "run-" + task.ID + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	r.record(runID, task.ID, trace.TaskCreated, nil)

	decision, debate, err := r.run(ctx, task, runID)

	r.record(runID, task.ID, trace.DecisionCreated, map[string]string{
		"status": string(decision.Status),
	})
	return decision, debate, err
}

// run выполняет задачу по режиму.
func (r *Runner) run(ctx context.Context, task domain.Task, runID string) (domain.Decision, *domain.Debate, error) {
	switch task.Mode {
	case domain.FAST:
		decision, err := r.runFast(ctx, task, runID)
		return decision, nil, err
	default:
		return r.runDebate(ctx, task, runID)
	}
}

// record пишет событие трассировки (best-effort).
func (r *Runner) record(runID, taskID string, eventType trace.EventType, metadata map[string]string) {
	ev, err := trace.NewEvent(runID, taskID, eventType)
	if err != nil {
		return
	}
	ev.Metadata = metadata
	_ = r.trace.Record(ev)
}

// runFast — одиночный участник без дебата (TASK-071).
// Решение одиночного участника оформляется как CONSENSUS: в FAST
// «консенсус» — это решение единственной модели (компромисс MVP,
// отдельно от NORMAL/DELIBERATE в benchmark).
func (r *Runner) runFast(ctx context.Context, task domain.Task, runID string) (domain.Decision, error) {
	ctxStart := time.Now()
	ctxText, err := r.contextBuilder.Build(task)
	if err != nil {
		return failedDecision(), fmt.Errorf("agent: контекст: %w", err)
	}
	r.record(runID, task.ID, trace.ContextBuilt, map[string]string{"duration_ms": strconv.FormatInt(time.Since(ctxStart).Milliseconds(), 10)})

	msgs := debate.ProposalPrompt(task, ctxText)
	if r.notify != nil {
		r.notify(r.fastParticipant.ID, debate.StagePropose)
	}
	resp, err := r.fastParticipant.LLM.Generate(ctx, llm.GenerationRequest{Messages: msgs})
	if err != nil {
		return failedDecision(), fmt.Errorf("agent: участник %s: %w", r.fastParticipant.ID, err)
	}

	proposal, err := debate.ParseProposal(resp.Content)
	if err != nil {
		return failedDecision(), fmt.Errorf("agent: участник %s: %w", r.fastParticipant.ID, err)
	}

	decision, err := domain.NewDecision(domain.Consensus, proposal.Decision, proposal.Confidence)
	if err != nil {
		return failedDecision(), fmt.Errorf("agent: решение: %w", err)
	}
	decision.SupportingArguments = proposal.Arguments
	decision.Risks = proposal.Risks
	return decision, nil
}

// runDebate — полный дебат через Debate Core.
func (r *Runner) runDebate(ctx context.Context, task domain.Task, runID string) (domain.Decision, *domain.Debate, error) {
	d, err := r.engine.Deliberate(ctx, task, runID)
	if err != nil {
		return failedDecision(), nil, fmt.Errorf("agent: дебат: %w", err)
	}
	if !d.HasDecision() {
		return failedDecision(), nil, fmt.Errorf("agent: дебат завершился без решения")
	}
	return *d.Decision, &d, nil
}

// failedDecision — решение со статусом FAILED.
func failedDecision() domain.Decision {
	d, _ := domain.NewDecision(domain.Failed, "", 0)
	return d
}