# Duo ex Arca

**Проект:** интеллектуальный персональный агент с двумя независимыми LLM
**Рабочее название:** **Duo ex Arca**
**Перевод:** «Двое из ларца»
**Версия:** 0.2
**Статус:** Architecture Draft

---

# 1. Назначение

Duo ex Arca — персональный интеллектуальный агент, в котором принятие существенных решений выполняется двумя независимыми LLM посредством внутреннего структурированного диалога.

Основная схема:

```text
User
  ↓
Task
  ↓
LLM A ──────┐
            │
            ├── independent proposals
            │
LLM B ──────┘
  ↓
cross critique
  ↓
revision
  ↓
consensus
  ↓
decision
  ↓
execution
  ↓
User
```

Главная идея проекта:

> две различные модели не просто генерируют два ответа, а проверяют и корректируют решения друг друга перед формированием итогового решения агента.
> язык жестко задается в конфигурации из списка поддерживаемых языков и далее: все внутренние размышления только на языке из настроек, всё взаимодействие с пользователем только на языке из настроек, все системные промпты только на языке из настрек
> базовый язык - русский

---

# 2. Жесткие архитектурные ограничения

## 2.1. Язык

Весь backend, runtime, CLI, orchestration, storage layer и тестовая инфраструктура MVP реализуются **только на Go**.

Не допускается:

- Python runtime;
- Node.js/TypeScript runtime;
- выполнение Python-кода как части архитектуры;
- зависимости от Python для запуска или сборки MVP.

Python может использоваться разработчиком отдельно для исследовательского анализа benchmark-данных, но не является частью продукта.

---

# 3. Хранилище

Основное persistent storage для MVP:

**SQLite**.

Причины:

- встроенная работа с одним файлом;
- отсутствие отдельного DB-сервера;
- хорошая поддержка Go;
- транзакции;
- SQL удобно использовать для task/debate/trace моделей;
- простое резервное копирование.

Архитектура storage должна оставаться абстрактной:

```text
Repository
    ↓
Storage interface
    ↓
SQLite
```

В будущем допускается Go-native key-value storage, например embedded KV engine, но переход к нему не должен требовать изменения domain layer.

Для первой реализации рекомендуется:

> **SQLite как единственное хранилище MVP.**

---

# 4. LLM Integration

Duo ex Arca **не содержит собственного inference runtime**.

LLM работают как внешние сервисы.

Примеры:

```text
OpenRouter
LM Studio
Ollama
llama.cpp server
облачные провайдеры
```

Для них используется стандартный HTTP API.

Основной контракт MVP:

**OpenAI-compatible Chat Completions API**.

Конкретный provider отличается в основном:

```text
base_url
model
api_key
headers
```

Например:

```text
https://openrouter.ai/api/v1
http://localhost:1234/v1
http://localhost:11434/v1
http://localhost:8080/v1
```

Таким образом:

```text
                 ┌─────────────────────┐
                 │     LLM Client      │
                 └──────────┬──────────┘
                            │
              OpenAI-compatible HTTP API
                            │
       ┌────────────┬───────┼──────────────┬────────────┐
       ↓            ↓       ↓              ↓
 OpenRouter      Ollama   LM Studio    llama.cpp    Cloud
```

---

# 5. LLM abstraction

Внутренний код не должен зависеть от названия OpenAI API.

Должен существовать собственный интерфейс:

```go
type LLM interface {
    Generate(ctx context.Context, request GenerationRequest) (
        GenerationResponse,
        error,
    )
}
```

Provider configuration:

```yaml
models:
  participant_a:
    base_url: "https://..."
    model: "..."
    api_key_env: "..."

  participant_b:
    base_url: "http://localhost:11434/v1"
    model: "..."
    api_key_env: ""
```

Таким образом, одна и та же модель клиента может работать:

```text
cloud → cloud
cloud → local
local → local
local → OpenRouter
OpenRouter → LM Studio
```

---

# 6. Почему не использовать SDK провайдеров

Duo ex Arca должен быть независим от экосистемы конкретного поставщика.

Поэтому в MVP запрещаются зависимости вида:

```text
openai-go SDK
anthropic SDK
ollama SDK
```

для основной LLM abstraction.

Причина проста:

> если базовый протокол единообразен, provider-specific SDK не дают архитектурного преимущества, но создают дополнительную связанность.

Provider-specific adapter может появиться позднее только при необходимости использования уникальных возможностей конкретного сервиса.

---

# 7. Основные компоненты

```text
┌──────────────────────────────────────────────┐
│                  CLI / API                   │
└──────────────────────┬───────────────────────┘
                       ↓
┌──────────────────────────────────────────────┐
│                 Agent Core                   │
└──────┬───────────────┬───────────────┬───────┘
       │               │               │
       ↓               ↓               ↓
 Context Builder   Debate Core      Tool Bus
                       │
                ┌──────┴──────┐
                ↓             ↓
             LLM A          LLM B
                │             │
                └──────┬──────┘
                       ↓
                 Consensus
                       ↓
                  Decision
                       ↓
                    Memory
                       ↓
                   SQLite
```

---

# 8. Debate Core

Debate Core является центральным компонентом системы.

Он реализует:

1. независимое первоначальное решение;
2. взаимную критику;
3. пересмотр решения;
4. оценку консенсуса;
5. повторение раундов при необходимости.

Основной протокол:

```text
Task
 ↓
Independent Proposal A
Independent Proposal B
 ↓
Critique A ← B
Critique B ← A
 ↓
Revision A
Revision B
 ↓
Consensus
```

---

# 9. Независимость участников

Критически важно, чтобы initial proposals не влияли друг на друга.

На первом этапе:

```text
Task
 ├────────→ LLM A
 │
 └────────→ LLM B
```

LLM A не видит proposal B.

LLM B не видит proposal A.

Только после завершения обоих independent proposals начинается debate.

---

# 10. Участники

Внутренне модели называются:

```text
Participant A
Participant B
```

Не следует заранее определять:

```text
A = thinker
B = critic
```

Оба участника симметричны.

Их роли меняются в ходе протокола.

---

# 11. Proposal

Предложение содержит:

```go
type Proposal struct {
    ID             string
    ParticipantID  string

    Decision       string
    Arguments      []string
    Assumptions    []string
    Risks          []string

    Confidence     float64
}
```

Структурированный результат предпочтительнее свободного текста.

---

# 12. Critique

```go
type Critique struct {
    ValidPoints        []string
    Errors             []string
    MissingInformation []string
    Risks              []string
    CounterArguments   []string
    ProposedChanges    []string
}
```

Критика должна заставлять модель искать причины ошибочности другого решения.

---

# 13. Decision

```go
type DecisionStatus string

const (
    Consensus          DecisionStatus = "CONSENSUS"
    Disagreement       DecisionStatus = "DISAGREEMENT"
    InsufficientData   DecisionStatus = "INSUFFICIENT_DATA"
    RequireUserInput   DecisionStatus = "REQUIRE_USER_INPUT"
    Failed             DecisionStatus = "FAILED"
)
```

Итог:

```go
type Decision struct {
    Status             DecisionStatus

    Decision           string
    Confidence         float64

    SupportingArguments []string
    RejectedArguments   []string

    Risks              []string
    UnresolvedIssues   []string

    Evidence           []Evidence
}
```

---

# 14. Consensus

Консенсус означает не совпадение текстов.

Он означает согласие по:

```text
основному решению
ключевым требованиям
существенным аргументам
критическим рискам
```

Если модели не согласны, система не должна искусственно выбирать победителя.

---

# 15. Режимы работы

```text
FAST
NORMAL
DELIBERATE
CRITICAL
```

### FAST

Одна LLM.

### NORMAL

Две LLM, один основной раунд.

### DELIBERATE

Две LLM, несколько раундов.

### CRITICAL

Две LLM + усиленная проверка + возможно внешний verifier.

Автоматический выбор режима не является обязательным для первой версии.

---

# 16. Model Pair

В каждом запуске у агента имеется:

```text
Participant A → provider/model
Participant B → provider/model
```

Модели могут быть:

```text
cloud / cloud
cloud / local
local / local
```

Желательно использовать различные model families, поскольку исследовательская ценность механизма зависит от независимости ошибок.

---

# 17. Context

Каждому участнику передается:

```text
Task
Constraints
Relevant Context
Evidence
Previous Decisions
```

Initial proposal дополнительно не содержит ответа другого участника.

---

# 18. Tool Bus

Инструменты не входят в первый MVP, однако архитектура должна предусматривать:

```text
LLM
 ↓
Tool Request
 ↓
Tool Bus
 ↓
Policy
 ↓
Tool
 ↓
Result
```

Ни один tool не должен напрямую исполнять необработанный текст модели.

---

# 19. Storage Architecture

```text
Domain
  ↓
Repository interfaces
  ↓
SQLite implementation
```

Основные сущности:

```text
tasks
debates
debate_rounds
proposals
critiques
decisions
traces
```

---

# 20. Trace

Каждый запуск должен иметь:

```text
trace_id
task_id
timestamp
event_type
participant
duration
metadata
```

Пример событий:

```text
TASK_CREATED
CONTEXT_BUILT
PROPOSAL_STARTED
PROPOSAL_COMPLETED
CRITIQUE_COMPLETED
REVISION_COMPLETED
CONSENSUS_EVALUATED
DECISION_CREATED
```

---

# 21. CLI

Минимальный интерфейс:

```bash
arca ask "Какую БД использовать?"
```

```bash
arca ask --mode deliberate "..."
```

```bash
arca ask --json "..."
```

```bash
arca trace <id>
```

---

# 22. Конфигурация

Пример:

```yaml
llm:
  participant_a:
    base_url: "https://openrouter.ai/api/v1"
    model: "..."
    api_key_env: "OPENROUTER_API_KEY"

  participant_b:
    base_url: "http://localhost:11434/v1"
    model: "..."
    api_key_env: ""

debate:
  default_mode: deliberate

  max_rounds:
    normal: 1
    deliberate: 3
    critical: 6

  consensus_threshold: 0.85

storage:
  type: sqlite
  path: "./arca.db"
```

---

# 23. Project structure

```text
duo-ex-arca/
├── cmd/
│   └── arca/
│       └── main.go
│
├── internal/
│   ├── domain/
│   │   ├── task.go
│   │   ├── proposal.go
│   │   ├── critique.go
│   │   ├── decision.go
│   │   └── debate.go
│   │
│   ├── llm/
│   │   ├── llm.go
│   │   ├── types.go
│   │   ├── client.go
│   │   └── mock.go
│   │
│   ├── debate/
│   │   ├── engine.go
│   │   ├── protocol.go
│   │   ├── prompts.go
│   │   └── consensus.go
│   │
│   ├── context/
│   │   └── builder.go
│   │
│   ├── agent/
│   │   └── runner.go
│   │
│   ├── storage/
│   │   ├── repository.go
│   │   └── sqlite/
│   │
│   ├── trace/
│   │   └── trace.go
│   │
│   └── config/
│       └── config.go
│
├── tests/
│   ├── unit/
│   └── integration/
│
├── benchmarks/
│
├── docs/
│
├── go.mod
└── README.md
```

---

# 24. Архитектурные инварианты

Следующие свойства должны сохраняться на протяжении всего проекта:

### I1 — Language

В runtime используется только Go.

### I2 — Storage

Domain не зависит от SQLite.

### I3 — LLM

Domain и Debate Core не знают конкретного LLM provider.

### I4 — Provider

LLM подключаются через стандартный HTTP API.

### I5 — Independence

Initial proposals независимы.

### I6 — No forced consensus

Disagreement является штатным результатом.

### I7 — Structured reasoning

Основные результаты LLM представлены структурированными объектами.

### I8 — Traceability

Каждое существенное решение имеет trace.

---

# 25. Основная исследовательская гипотеза

Duo ex Arca должен экспериментально проверить:

> две независимые LLM, проходящие через structured debate, могут повышать надежность решения по сравнению с одной LLM.

MVP должен быть прежде всего платформой для проверки этой гипотезы.