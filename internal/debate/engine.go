package debate

import (
	"context"
	"fmt"
	"strconv"
	"time"

	ctxb "github.com/radamsa/duo-ex-arca/internal/context"
	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/llm"
	"github.com/radamsa/duo-ex-arca/internal/trace"
)

// Participant связывает идентификатор участника с его LLM
// (docs/plan-mvp.md, TASK-050). Участники симметричны: роли
// предложения/критика меняются в ходе протокола.
type Participant struct {
	ID  string
	LLM llm.LLM
}

// NewParticipant создаёт участника дебата.
func NewParticipant(id string, model llm.LLM) *Participant {
	return &Participant{ID: id, LLM: model}
}

// Engine — Debate Core: протокол независимых предложений,
// взаимной критики, пересмотра и оценки консенсуса.
type Engine struct {
	participantA  *Participant
	participantB  *Participant
	contextBuilder *ctxb.Builder
	consensus      *ConsensusEngine
	maxRounds      map[domain.TaskMode]int
	trace          trace.Recorder
}

// EngineConfig — конфигурация движка.
type EngineConfig struct {
	ParticipantA *Participant
	ParticipantB *Participant

	ContextBuilder *ctxb.Builder

	// ConsensusThreshold — порог уверенности консенсуса (0,1].
	ConsensusThreshold float64

	// MaxRounds — лимит раундов по режимам (кроме FAST,
	// который движок не обслуживает).
	MaxRounds map[domain.TaskMode]int

	// Trace — рекордер событий трассировки (опционально;
	// при nil события не записываются).
	Trace trace.Recorder
}

// NewEngine создаёт движок и проверяет конфигурацию.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	if cfg.Trace == nil {
		cfg.Trace = trace.Noop{}
	}
	if cfg.ParticipantA == nil || cfg.ParticipantA.LLM == nil {
		return nil, fmt.Errorf("debate: не задан участник A")
	}
	if cfg.ParticipantB == nil || cfg.ParticipantB.LLM == nil {
		return nil, fmt.Errorf("debate: не задан участник B")
	}
	if cfg.ContextBuilder == nil {
		return nil, fmt.Errorf("debate: не задан ContextBuilder")
	}

	consensus, err := NewConsensusEngine(cfg.ConsensusThreshold)
	if err != nil {
		return nil, err
	}

	if cfg.MaxRounds == nil {
		return nil, fmt.Errorf("debate: не заданы лимиты раундов MaxRounds")
	}
	for _, mode := range []domain.TaskMode{domain.NORMAL, domain.DELIBERATE, domain.CRITICAL} {
		if cfg.MaxRounds[mode] < 1 {
			return nil, fmt.Errorf("debate: не задан лимит раундов для режима %s", mode)
		}
	}

	return &Engine{
		participantA:   cfg.ParticipantA,
		participantB:   cfg.ParticipantB,
		contextBuilder: cfg.ContextBuilder,
		consensus:      consensus,
		maxRounds:      cfg.MaxRounds,
		trace:          cfg.Trace,
	}, nil
}

// Deliberate проводит дебат по задаче и возвращает дебат с решением.
// runID — идентификатор запуска для событий трассировки.
//
// Протокол одного раунда (docs/design.md, раздел 8):
//  1. независимые initial proposals (параллельно);
//  2. взаимная критика (A критикует B, B критикует A, параллельно);
//  3. пересмотр собственных решений (параллельно);
//  4. оценка консенсуса обоими участниками (параллельно);
//  5. при консенсусе или нехватке данных — ранний выход (TASK-056).
func (e *Engine) Deliberate(ctx context.Context, task domain.Task, runID string) (domain.Debate, error) {
	if task.Mode == domain.FAST {
		return domain.Debate{}, fmt.Errorf("debate: режим FAST обслуживается не движком дебата, а одним участником")
	}

	d, err := domain.NewDebate("debate-"+task.ID, task)
	if err != nil {
		return domain.Debate{}, err
	}

	ctxStart := time.Now()
	ctxText, err := e.contextBuilder.Build(task)
	if err != nil {
		return domain.Debate{}, fmt.Errorf("debate: не удалось построить контекст: %w", err)
	}
	e.record(runID, task.ID, trace.ContextBuilt, "", time.Since(ctxStart), nil)

	maxRounds := e.maxRounds[task.Mode]
	for roundNum := 1; roundNum <= maxRounds; roundNum++ {
		round, err := e.runRound(ctx, task, ctxText, roundNum, runID)
		if err != nil {
			return domain.Debate{}, err
		}
		if err := d.AddRound(round); err != nil {
			return domain.Debate{}, err
		}

		consensusStart := time.Now()
		verdictA, verdictB, err := e.evaluateConsensusParallel(ctx, task, round)
		if err != nil {
			return domain.Debate{}, err
		}

		decision, err := e.consensus.Evaluate(verdictA, verdictB)
		if err != nil {
			return domain.Debate{}, err
		}
		d.SetDecision(decision)

		e.record(runID, task.ID, trace.ConsensusEvaluated, "",
			time.Since(consensusStart), map[string]string{
				"agreement": string(decision.Status),
				"round":     strconv.Itoa(roundNum),
			})

		// Ранний выход: консенсус достигнут или данных не хватает.
		if decision.Status == domain.Consensus || decision.Status == domain.InsufficientData {
			break
		}
	}

	return d, nil
}

// record пишет событие трассировки (best-effort: ошибки игнорируются,
// чтобы сбой трассировки не ломал дебат).
func (e *Engine) record(runID, taskID string, eventType trace.EventType, participant string, duration time.Duration, metadata map[string]string) {
	ev, err := trace.NewEvent(runID, taskID, eventType)
	if err != nil {
		return
	}
	ev.Participant = participant
	ev.Duration = duration
	ev.Metadata = metadata
	_ = e.trace.Record(ev)
}

// runRound выполняет фазы proposal -> critique -> revision одного раунда,
// записывая события трассировки каждой фазы.
func (e *Engine) runRound(ctx context.Context, task domain.Task, ctxText string, roundNum int, runID string) (domain.DebateRound, error) {
	r, err := domain.NewDebateRound(roundNum)
	if err != nil {
		return domain.DebateRound{}, err
	}

	e.record(runID, task.ID, trace.ProposalStarted, "", 0, map[string]string{"round": strconv.Itoa(roundNum)})
	phaseStart := time.Now()
	pa, pb, err := e.proposePair(ctx, task, ctxText, roundNum)
	if err != nil {
		return domain.DebateRound{}, err
	}
	e.record(runID, task.ID, trace.ProposalCompleted, "", time.Since(phaseStart), nil)
	r.ProposalA, r.ProposalB = &pa, &pb

	e.record(runID, task.ID, trace.CritiqueStarted, "", 0, map[string]string{"round": strconv.Itoa(roundNum)})
	phaseStart = time.Now()
	ca, cb, err := e.critiquePair(ctx, task, ctxText, pa, pb)
	if err != nil {
		return domain.DebateRound{}, err
	}
	e.record(runID, task.ID, trace.CritiqueCompleted, "", time.Since(phaseStart), nil)
	r.CritiqueA, r.CritiqueB = &ca, &cb

	phaseStart = time.Now()
	ra, rb, err := e.revisePair(ctx, task, ctxText, pa, pb, ca, cb, roundNum)
	if err != nil {
		return domain.DebateRound{}, err
	}
	e.record(runID, task.ID, trace.RevisionCompleted, "", time.Since(phaseStart), nil)
	r.RevisionA, r.RevisionB = &ra, &rb

	return r, nil
}

// propose выполняет для участника Initial Proposal (TASK-051).
func (e *Engine) propose(ctx context.Context, p *Participant, task domain.Task, ctxText string, side string, roundNum int) (domain.Proposal, error) {
	msgs := ProposalPrompt(task, ctxText)
	proposalObj, err := e.generateProposal(ctx, p, msgs)
	if err != nil {
		return domain.Proposal{}, fmt.Errorf("debate: участник %s (proposal): %w", p.ID, err)
	}
	proposalObj.ID = fmt.Sprintf("%s-%d", side, roundNum)
	proposalObj.ParticipantID = p.ID
	return proposalObj, nil
}

// generateProposal — единая точка генерации и разбора предложений.
func (e *Engine) generateProposal(ctx context.Context, p *Participant, msgs []llm.Message) (domain.Proposal, error) {
	resp, err := p.LLM.Generate(ctx, llm.GenerationRequest{Messages: msgs})
	if err != nil {
		return domain.Proposal{}, err
	}
	return ParseProposal(resp.Content)
}

// critique выполняет для участника критику предложения оппонента (TASK-052).
func (e *Engine) critique(ctx context.Context, p *Participant, task domain.Task, ctxText string, target domain.Proposal) (domain.Critique, error) {
	msgs := CritiquePrompt(task, target, ctxText)
	resp, err := p.LLM.Generate(ctx, llm.GenerationRequest{Messages: msgs})
	if err != nil {
		return domain.Critique{}, fmt.Errorf("debate: участник %s (critique): %w", p.ID, err)
	}
	c, err := ParseCritique(resp.Content)
	if err != nil {
		return domain.Critique{}, fmt.Errorf("debate: участник %s (critique): %w", p.ID, err)
	}
	return c, nil
}

// revise выполняет для участника пересмотр своего предложения (TASK-053).
func (e *Engine) revise(ctx context.Context, p *Participant, task domain.Task, ctxText string, own domain.Proposal, peerCritique domain.Critique, side string, roundNum int) (domain.Proposal, error) {
	msgs := RevisionPrompt(task, own, peerCritique, ctxText)
	revisionObj, err := e.generateProposal(ctx, p, msgs)
	if err != nil {
		return domain.Proposal{}, fmt.Errorf("debate: участник %s (revision): %w", p.ID, err)
	}
	revisionObj.ID = fmt.Sprintf("%sr-%d", side, roundNum)
	revisionObj.ParticipantID = p.ID
	return revisionObj, nil
}

// evaluateConsensusParallel опрашивает обоих участников как арбитров (TASK-033).
func (e *Engine) evaluateConsensusParallel(ctx context.Context, task domain.Task, round domain.DebateRound) (ConsensusVerdict, ConsensusVerdict, error) {
	msgs := ConsensusPrompt(*round.ProposalA, *round.ProposalB, *round.RevisionA, *round.RevisionB, task.Constraints)

	evaluate := func(p *Participant) (ConsensusVerdict, error) {
		resp, err := p.LLM.Generate(ctx, llm.GenerationRequest{Messages: msgs})
		if err != nil {
			return ConsensusVerdict{}, fmt.Errorf("debate: участник %s (consensus): %w", p.ID, err)
		}
		v, err := ParseConsensusVerdict(resp.Content)
		if err != nil {
			return ConsensusVerdict{}, fmt.Errorf("debate: участник %s (consensus): %w", p.ID, err)
		}
		return v, nil
	}

	return runBoth(ctx, func() (ConsensusVerdict, error) { return evaluate(e.participantA) },
		func() (ConsensusVerdict, error) { return evaluate(e.participantB) })
}

// proposePair запускает независимые предложения параллельно.
func (e *Engine) proposePair(ctx context.Context, task domain.Task, ctxText string, roundNum int) (domain.Proposal, domain.Proposal, error) {
	return runBoth(ctx,
		func() (domain.Proposal, error) { return e.propose(ctx, e.participantA, task, ctxText, "a", roundNum) },
		func() (domain.Proposal, error) { return e.propose(ctx, e.participantB, task, ctxText, "b", roundNum) })
}

// critiquePair запускает взаимную критику параллельно.
func (e *Engine) critiquePair(ctx context.Context, task domain.Task, ctxText string, pa, pb domain.Proposal) (domain.Critique, domain.Critique, error) {
	return runBoth(ctx,
		func() (domain.Critique, error) { return e.critique(ctx, e.participantA, task, ctxText, pb) },
		func() (domain.Critique, error) { return e.critique(ctx, e.participantB, task, ctxText, pa) })
}

// revisePair запускает пересмотры параллельно.
func (e *Engine) revisePair(ctx context.Context, task domain.Task, ctxText string, pa, pb domain.Proposal, ca, cb domain.Critique, roundNum int) (domain.Proposal, domain.Proposal, error) {
	return runBoth(ctx,
		func() (domain.Proposal, error) { return e.revise(ctx, e.participantA, task, ctxText, pa, cb, "a", roundNum) },
		func() (domain.Proposal, error) { return e.revise(ctx, e.participantB, task, ctxText, pb, ca, "b", roundNum) })
}

// pairResult — результат параллельного выполнения с индексом стороны.
type pairResult[T any] struct {
	index int
	value T
	err   error
}

// runBoth выполняет две функции параллельно и возвращает ошибку
// первой завершившейся неудачей (аналог errgroup, только stdlib).
func runBoth[T any](ctx context.Context, fa, fb func() (T, error)) (T, T, error) {
	ch := make(chan pairResult[T], 2)

	go func() {
		v, err := fa()
		ch <- pairResult[T]{index: 0, value: v, err: err}
	}()
	go func() {
		v, err := fb()
		ch <- pairResult[T]{index: 1, value: v, err: err}
	}()

	values := make([]T, 2)
	firstErr := true
	var errOut error
	for i := 0; i < 2; i++ {
		res := <-ch
		values[res.index] = res.value
		if res.err != nil && firstErr {
			errOut = res.err
			firstErr = false
		}
	}
	if errOut != nil {
		var zero T
		return zero, zero, errOut
	}
	return values[0], values[1], nil
}