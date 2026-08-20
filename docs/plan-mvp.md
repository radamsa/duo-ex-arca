# Duo ex Arca — подробный план разработки MVP

## 1. Стратегия

Разработка выполняется последовательно небольшими work items, пригодными для выполнения coding-agent.

Каждый work item должен:

- иметь одну четкую цель;
- менять ограниченное количество файлов;
- иметь acceptance criteria;
- содержать тесты;
- не ломать предыдущую архитектуру.

---

# 2. Технологический baseline

Обязательный стек:

```text
Go
SQLite
HTTP/JSON
OpenAI-compatible LLM API
```

Рекомендуемые Go-компоненты:

```text
Go standard library
modernc.org/sqlite
```

SQLite driver следует выбрать такой, чтобы проект не требовал системного C toolchain.

В LLM layer используется стандартная библиотека `net/http`, а не SDK конкретных провайдеров.

---

# 3. Этап 0 — Repository

### TASK-001 — Create Go repository

Создать:

```text
go.mod
cmd/arca/
internal/
tests/
docs/
README.md
.gitignore
```

Добавить минимальный `main.go`.

Acceptance:

```bash
go build ./...
go test ./...
```

---

### TASK-002 — Development baseline

Настроить:

```bash
go test ./...
go vet ./...
```

При необходимости добавить `golangci-lint`.

Acceptance:

все проверки проходят.

---

# 4. Этап 1 — Domain

### TASK-003 — Task

Создать:

```go
Task
TaskMode
```

Поддержать:

```text
FAST
NORMAL
DELIBERATE
CRITICAL
```

---

### TASK-004 — Proposal

Реализовать `Proposal`.

---

### TASK-005 — Critique

Реализовать `Critique`.

---

### TASK-006 — Decision

Реализовать:

```text
Decision
DecisionStatus
```

---

### TASK-007 — Debate

Реализовать:

```text
Debate
DebateRound
```

На этом этапе никаких LLM.

Цель — получить полностью тестируемую domain model.

---

# 5. Этап 2 — LLM abstraction

### TASK-010 — LLM interface

Создать:

```go
type LLM interface {
    Generate(
        context.Context,
        GenerationRequest,
    ) (GenerationResponse, error)
}
```

---

### TASK-011 — LLM types

Создать:

```text
Message
Role
GenerationRequest
GenerationResponse
Usage
```

---

### TASK-012 — OpenAI-compatible HTTP client

Реализовать общий HTTP client.

Он должен работать только с:

```text
base URL
model
API key
```

Основные HTTP операции:

```text
POST /chat/completions
```

---

### TASK-013 — Error model

Нормализовать:

```text
timeout
connection error
HTTP error
rate limit
invalid response
invalid JSON
context overflow
```

---

### TASK-014 — Mock LLM

Создать deterministic mock.

Он должен позволять тестировать:

```text
proposal
critique
revision
consensus
failure
timeout
```

---

# 6. Этап 3 — Provider configuration

### TASK-020 — Config model

Создать:

```text
Config
LLMConfig
ParticipantConfig
DebateConfig
StorageConfig
```

---

### TASK-021 — YAML/JSON configuration

Загрузить конфигурацию из файла.

Например:

```yaml
llm:
  participant_a:
    base_url: "https://..."
    model: "..."

  participant_b:
    base_url: "http://localhost:11434/v1"
    model: "..."
```

---

### TASK-022 — Environment secrets

API key никогда не хранится непосредственно в config.

Только:

```text
api_key_env
```

---

# 7. Этап 4 — Prompt protocol

### TASK-030 — Proposal prompt

Создать отдельный builder.

Обязательные поля:

```text
decision
arguments
assumptions
risks
confidence
```

---

### TASK-031 — Critique prompt

Обязательные поля:

```text
valid_points
errors
missing_information
risks
counter_arguments
proposed_changes
```

---

### TASK-032 — Revision prompt

Передавать:

```text
task
own proposal
critique
context
```

---

### TASK-033 — Consensus prompt

Передавать:

```text
proposal A
proposal B
revision A
revision B
requirements
```

---

# 8. Этап 5 — Context Builder

### TASK-040

Реализовать:

```go
ContextBuilder
```

На MVP:

```text
Task
Constraints
```

---

### TASK-041 — Context isolation tests

Обеспечить:

```text
initial A does not contain B
initial B does not contain A
```

Это отдельный обязательный тест.

---

# 9. Этап 6 — Debate Engine

### TASK-050 — Participant

Создать abstraction:

```go
Participant
```

Participant связывает:

```text
participant id
LLM
prompt builder
```

---

### TASK-051 — Independent proposals

Реализовать параллельный вызов:

```go
proposalA, proposalB
```

Через:

```go
errgroup
```

или аналогичный механизм стандартной concurrency модели Go.

---

### TASK-052 — Critique phase

Параллельно:

```text
A critiques B
B critiques A
```

---

### TASK-053 — Revision phase

Параллельно:

```text
A revises
B revises
```

---

### TASK-054 — Debate round

Собрать полный round:

```text
proposal
critique
revision
```

---

### TASK-055 — Multiple rounds

Добавить цикл:

```text
for round := 1; round <= maxRounds; round++
```

---

### TASK-056 — Early termination

Если consensus достигнут:

```text
break
```

---

# 10. Этап 7 — Consensus Engine

### TASK-060

Создать:

```go
ConsensusEngine
```

---

### TASK-061 — Consensus evaluation

Проверять:

```text
decision
requirements
arguments
risks
confidence
```

---

### TASK-062 — Disagreement

Добавить явную обработку:

```text
DISAGREEMENT
```

---

### TASK-063 — Insufficient data

Если обе модели указывают на отсутствие необходимых данных:

```text
INSUFFICIENT_DATA
```

---

# 11. Этап 8 — Agent Runner

### TASK-070

Создать:

```go
AgentRunner
```

Pipeline:

```text
Task
 ↓
Context
 ↓
Debate
 ↓
Decision
```

---

### TASK-071 — Modes

Поддержать:

```text
FAST
NORMAL
DELIBERATE
CRITICAL
```

FAST может временно обходить Debate Core и использовать одного participant.

---

# 12. Этап 9 — SQLite

### TASK-080 — SQLite initialization

Создать DB:

```text
arca.db
```

Автоматическая инициализация schema.

---

### TASK-081 — Tasks table

---

### TASK-082 — Debates table

---

### TASK-083 — Rounds table

---

### TASK-084 — Decisions table

---

### TASK-085 — Trace table

---

### TASK-086 — Repository interfaces

Domain не должен знать SQL.

---

# 13. Этап 10 — Trace

### TASK-090

Создать trace model.

События:

```text
TASK_CREATED
CONTEXT_BUILT
PROPOSAL_STARTED
PROPOSAL_COMPLETED
CRITIQUE_STARTED
CRITIQUE_COMPLETED
REVISION_COMPLETED
CONSENSUS_EVALUATED
DECISION_CREATED
```

---

### TASK-091 — Trace persistence

Сохранять trace в SQLite.

---

# 14. Этап 11 — CLI

### TASK-100

```bash
arca ask "..."
```

---

### TASK-101

```bash
arca ask --mode normal "..."
arca ask --mode deliberate "..."
```

---

### TASK-102

```bash
arca ask --json "..."
```

---

### TASK-103

```bash
arca trace <task-id>
```

---

### TASK-104

```bash
arca config
arca health
```

---

# 15. Этап 12 — Error handling

### TASK-110

Добавить timeout для каждого LLM request.

---

### TASK-111

Добавить retry только для retryable errors.

---

### TASK-112

Нормализовать provider errors.

---

### TASK-113

Обработать частичный отказ одного участника.

Не создавать ложный consensus.

---

# 16. Этап 13 — Real providers

Порядок:

```text
Mock
 ↓
один реальный OpenAI-compatible endpoint
 ↓
второй endpoint
 ↓
разные model families
```

---

### TASK-120 — OpenRouter

Проверить работу общего OpenAI-compatible client.

Не создавать отдельный SDK adapter.

---

### TASK-121 — Ollama

Подключить через изменение:

```text
base_url
model
```

без изменения Debate Core.

---

### TASK-122 — LM Studio

Аналогично.

---

### TASK-123 — llama.cpp server

Аналогично.

---

# 17. Важное архитектурное требование provider layer

После выполнения TASK-120..123 код Debate Core должен выглядеть одинаково для всех:

```go
participantA := NewParticipant(config.ParticipantA)
participantB := NewParticipant(config.ParticipantB)

decision, err := engine.Deliberate(ctx, task)
```

Никакого:

```go
if provider == "ollama" ...
if provider == "openrouter" ...
```

в Debate Core быть не должно.

---

# 18. Этап 14 — Unit Tests

### TASK-130

Domain tests.

---

### TASK-131

LLM client tests через `httptest.Server`.

Это особенно важно.

Не использовать реальные сервисы для unit tests.

---

### TASK-132

Debate Engine tests с Mock LLM.

---

### TASK-133

Consensus tests.

---

### TASK-134

Context isolation tests.

---

# 19. Этап 15 — Integration Tests

### TASK-140

Полный pipeline:

```text
CLI
→ AgentRunner
→ DebateEngine
→ Mock LLM
→ SQLite
→ Decision
```

---

### TASK-141

Failure scenarios:

```text
A timeout
B timeout
A invalid JSON
B HTTP 500
```

---

# 20. Этап 16 — Benchmark

### TASK-150 — Dataset format

JSONL:

```json
{
  "id": "001",
  "category": "reasoning",
  "task": "...",
  "expected": "..."
}
```

---

### TASK-151 — Single LLM baseline

```text
one LLM
```

---

### TASK-152 — Duo mode

```text
LLM A
+
LLM B
+
debate
```

---

### TASK-153 — Result persistence

Каждый benchmark run сохраняет:

```text
task_id
mode
models
rounds
latency
tokens
result
score
```

---

# 21. Этап 17 — Evaluators

Приоритет:

```text
deterministic evaluator
```

перед LLM evaluator.

Например:

```text
coding → go test
JSON → schema validation
SQL → execution
exact answer → comparison
```

LLM judge использовать только там, где deterministic evaluation невозможна.

---

# 22. Этап 18 — Benchmark experiments

Минимальные конфигурации:

```text
A = Model A only
B = Model B only
C = Model A + self critique
D = Model A + Model B debate
```

Сравнить:

```text
accuracy
success rate
latency
tokens
cost
rounds
```

---

# 23. Этап 19 — Optimization

Только после benchmark:

```text
parallel initial proposals
early consensus
adaptive rounds
prompt compression
context reduction
```

Первичная цель оптимизации:

> минимизировать latency и стоимость без падения качества.

---

# 24. Этап 20 — Final MVP

Перед релизом пройти:

```text
go build ./...
go test ./...
go vet ./...
```

Проверить:

```text
[ ] две реальные LLM
[ ] OpenAI-compatible API
[ ] OpenRouter
[ ] Ollama
[ ] LM Studio
[ ] llama.cpp
[ ] SQLite
[ ] CLI
[ ] JSON output
[ ] trace
[ ] debate
[ ] consensus
[ ] disagreement
[ ] benchmark
```

---

# 25. Итоговый порядок work items

```text
001-002   Repository/tooling

003-007   Domain

010-014   LLM abstraction

020-022   Configuration

030-033   Prompt protocol

040-041   Context

050-056   Debate Engine

060-063   Consensus

070-071   Agent Runner

080-086   SQLite

090-091   Trace

100-104   CLI

110-113   Error handling

120-123   Real LLM endpoints

130-134   Unit tests

140-141   Integration tests

150-153   Benchmark

160+      Optimization and finalization
```

---

# 26. Главное правило для Codex

Каждый новый work item обязан сохранять следующие инварианты:

```text
Go only
SQLite only for MVP persistence
HTTP/JSON for LLM
OpenAI-compatible API
No provider SDK
No provider-specific logic in Debate Core
Independent initial proposals
Explicit consensus
Explicit disagreement
Tests for new behavior
```

---

# 27. Предлагаемый первый набор задач для Codex

На практике разработку стоит начать не с LLM, а с четырех маленьких задач:

```text
TASK-001
Создать Go repository и минимальный CLI.

TASK-002
Создать domain model Task / Proposal / Critique / Decision / Debate.

TASK-003
Создать LLM interface и OpenAI-compatible HTTP client.

TASK-004
Создать deterministic Mock LLM и tests.
```

После этого уже можно безопасно отдавать Codex реализацию `DebateEngine`.

Это существенно уменьшает риск получить большой, но плохо тестируемый prototype.