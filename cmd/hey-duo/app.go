// Сборка полного pipeline агента из конфигурации (TASK-103).
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/radamsa/duo-ex-arca/internal/agent"
	"github.com/radamsa/duo-ex-arca/internal/config"
	ctxb "github.com/radamsa/duo-ex-arca/internal/context"
	"github.com/radamsa/duo-ex-arca/internal/debate"
	"github.com/radamsa/duo-ex-arca/internal/domain"
	"github.com/radamsa/duo-ex-arca/internal/llm"
	"github.com/radamsa/duo-ex-arca/internal/storage/sqlite"
	"github.com/radamsa/duo-ex-arca/internal/trace"
)

// defaultLLMTimeout — таймаут HTTP-запроса к LLM, если в конфиге 0.
// Единственный источник истины по умолчанию — config.Default().
var defaultLLMTimeout = time.Duration(config.Default().LLM.TimeoutSeconds) * time.Second

// llmTimeout возвращает таймаут запроса к LLM из конфигурации.
// 0 в конфиге означает значение по умолчанию.
func llmTimeout(cfg config.Config) time.Duration {
	if cfg.LLM.TimeoutSeconds > 0 {
		return time.Duration(cfg.LLM.TimeoutSeconds) * time.Second
	}
	return defaultLLMTimeout
}

// newClient создаёт LLM-клиент участника с таймаутом из конфигурации
// и ретраями для retryable ошибок.
func newClient(p config.ParticipantConfig, timeout time.Duration) *llm.Client {
	return llm.NewClient(p.BaseURL, p.Model, p.APIKey(),
		llm.WithHTTPClient(&http.Client{Timeout: timeout}),
		llm.WithRetries(3),
	)
}

// maybeDebug в режиме --dev оборачивает клиента в отладочный декоратор,
// пишущий полные промпты и ответы модели в stderr.
func maybeDebug(label string, inner *llm.Client, dev bool) llm.LLM {
	if !dev {
		return inner
	}
	return llm.NewDebugClient(label, os.Stderr, inner)
}

// app — собранный агент: runner и репозитории.
type app struct {
	runner    *agent.Runner
	db        *sqlite.DB
	tasks     *sqlite.TaskRepository
	debates   *sqlite.DebateRepository
	traces    *sqlite.TraceRepository
	benchRepo *sqlite.BenchmarkRepository
	cfg       config.Config
	activity  *activityReporter
}

// buildApp собирает pipeline: клиенты -> движок -> runner -> хранилище.
func buildApp(cfg config.Config, dev bool) (*app, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	participantA := debate.NewParticipant("participant-a", maybeDebug("participant-a", newClient(cfg.LLM.ParticipantA, llmTimeout(cfg)), dev))
	participantB := debate.NewParticipant("participant-b", maybeDebug("participant-b", newClient(cfg.LLM.ParticipantB, llmTimeout(cfg)), dev))
	contextBuilder := ctxb.New()

	db, err := sqlite.Open(cfg.Storage.Path)
	if err != nil {
		return nil, err
	}
	closeOnError := func(e error) (*app, error) {
		_ = db.Close()
		return nil, e
	}

	traceRepo := sqlite.NewTraceRepository(db)
	recorder := newSQLiteRecorder(traceRepo)

	// Спиннер активности не нужен в режиме --dev: там весь ход дебата
	// и так виден в stderr.
	var activity *activityReporter
	var notify debate.NotifyFunc
	if !dev {
		activity = newActivityReporter(os.Stdout)
		notify = activity.set
	}

	engine, err := debate.NewEngine(debate.EngineConfig{
		ParticipantA:        participantA,
		ParticipantB:        participantB,
		ContextBuilder:      contextBuilder,
		ConsensusThreshold:  cfg.Debate.ConsensusThreshold,
		SimilarityThreshold: cfg.Debate.SimilarityThreshold,
		MaxRounds:           modeRounds(cfg.Debate.MaxRounds),
		Trace:               recorder,
		Notify:              notify,
	})
	if err != nil {
		return closeOnError(err)
	}

	runner, err := agent.NewRunner(agent.RunnerConfig{
		Engine:          engine,
		FastParticipant: participantA,
		ContextBuilder:  contextBuilder,
		Trace:           recorder,
		Notify:          notify,
	})
	if err != nil {
		return closeOnError(err)
	}

	return &app{
		runner:   runner,
		db:       db,
		tasks:    sqlite.NewTaskRepository(db),
		debates:  sqlite.NewDebateRepository(db),
		traces:   traceRepo,
		cfg:      cfg,
		activity: activity,
	}, nil
}

// modeNameToMode переводит имя режима из конфигурации/флага в доменное значение.
func modeNameToMode(name string) (domain.TaskMode, bool) {
	switch name {
	case "fast":
		return domain.FAST, true
	case "normal":
		return domain.NORMAL, true
	case "deliberate":
		return domain.DELIBERATE, true
	case "critical":
		return domain.CRITICAL, true
	default:
		return "", false
	}
}

// modeRounds заполняет лимиты раундов по режимам, подставляя
// значения по умолчанию для отсутствующих ключей конфигурации.
func modeRounds(configured map[string]int) map[domain.TaskMode]int {
	defaults := config.Default().Debate.MaxRounds
	result := make(map[domain.TaskMode]int, 3)
	for _, mode := range []domain.TaskMode{domain.NORMAL, domain.DELIBERATE, domain.CRITICAL} {
		switch mode {
		case domain.NORMAL:
			result[mode] = firstPositive(configured["normal"], defaults["normal"])
		case domain.DELIBERATE:
			result[mode] = firstPositive(configured["deliberate"], defaults["deliberate"])
		case domain.CRITICAL:
			result[mode] = firstPositive(configured["critical"], defaults["critical"])
		}
	}
	return result
}

// firstPositive возвращает первое положительное значение.
func firstPositive(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 1
}

// sqliteRecorder адаптирует TraceRepository к trace.Recorder.
// Ошибки записи игнорируются: трассировка не должна ломать дебат.
type sqliteRecorder struct {
	repo *sqlite.TraceRepository
	ctx  context.Context
}

// newSQLiteRecorder создаёт рекордер с фоновым контекстом.
func newSQLiteRecorder(repo *sqlite.TraceRepository) *sqliteRecorder {
	return &sqliteRecorder{repo: repo, ctx: context.Background()}
}

// Record сохраняет событие трассировки (best-effort).
func (r *sqliteRecorder) Record(event trace.Event) error {
	return r.repo.Append(r.ctx, event)
}