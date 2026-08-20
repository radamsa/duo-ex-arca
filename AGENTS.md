# AGENTS.md

## Проект

Duo ex Arca — персональный агент, в котором решение принимают две независимые LLM через структурированный дебат (proposal → cross critique → revision → consensus). Цель — проверить гипотезу «две LLM с дебатом надёжнее одной».

## Состояние репозитория

- Кода пока нет. Источник истины по архитектуре — `docs/design.md` (инварианты I1–I8, структура каталогов).
- Разработка идёт по `docs/plan-mvp.md`: work items TASK-001..160+, порядок обязателен. Каждый item = одна цель, ограниченный набор файлов, acceptance criteria, тесты.

## Язык

Всё на русском: комментарии в коде, доки, сообщения, промпты. Базовый язык агента — русский.

## Стек (жёсткие ограничения)

- Go только. Никаких Python/Node.js runtime в продукте.
- SQLite через `modernc.org/sqlite` — без системного C toolchain.
- LLM — через HTTP/JSON по OpenAI-compatible Chat Completions API, только стандартный `net/http`. Запрещены SDK провайдеров (openai-go, anthropic, ollama SDK).
- Никакой provider-specific логики (`if provider == "ollama"` ...) в Debate Core.

## Инварианты

- Domain не знает SQLite и LLM-провайдеров: Repository interfaces → SQLite implementation.
- Initial proposals независимы: контекст A не содержит ответа B и наоборот (обязательный тест TASK-041).
- Consensus — это согласие по решению/требованиям/аргументам/рискам, не совпадение текстов. Disagreement и InsufficientData — штатные результаты; запрещён искусственный выбор победителя.
- Результаты LLM — структурированные объекты (Proposal, Critique, Decision), не свободный текст.
- Каждое существенное решение имеет trace.
- Новое поведение обязано иметь тесты. Unit-тесты LLM — только через `httptest.Server`, никаких реальных сервисов.

## Структура (задана в design.md)

```text
cmd/arca/                  CLI (ask, trace, config, health)
internal/domain/           Task, Proposal, Critique, Decision, Debate
internal/llm/              интерфейс, типы, OpenAI-compatible клиент, mock
internal/debate/           engine, protocol, prompts, consensus
internal/context/          ContextBuilder
internal/agent/            AgentRunner
internal/storage/sqlite/   репозитории
internal/trace/            события trace
internal/config/           конфиг (YAML/JSON + api_key_env, ключи в файл не класть)
tests/unit/ tests/integration/   benchmarks/
```

## Проверка

```bash
go build ./...
go test ./...
go vet ./...
```

`golangci-lint` — только если понадобится (в плане «при необходимости»). Acceptance каждого TASK включает прохождение этих команд.

## Workflow

- Работать через локальные скиллы `.agents/skills/`: `implement`/`tdd` — реализация задач, `to-tickets` — декомпозиция, `handoff` — передача контекста, `code-review` — ревью изменений.
- API-ключи — только через `api_key_env` в конфиге, никогда в файлы конфигурации.
