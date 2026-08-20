# Makefile для Duo ex Arca
#
# Целевые команды:
#   make setup  — подготовка окружения (модули, инструменты)
#   make build  — сборка приложения в бинарник hey-duo
#   make test   — запуск всех тестов
#   make lint   — запуск линтера (golangci-lint)
#   make clean  — очистка артефактов сборки
#   make dev    — запуск приложения в режиме разработчика

# Имя исполняемого файла.
BINARY := hey-duo
# Путь к главному пакету.
CMD := ./cmd/hey-duo

# modernc.org/sqlite зафиксирован под Go 1.22.2;
# GOTOOLCHAIN=local запрещает авто-подкачку новых тулчейнов.
GO := GOTOOLCHAIN=local go

.PHONY: setup build test lint clean dev

## setup: загружает зависимости и инструменты разработки.
setup:
	$(GO) mod download
	@command -v golangci-lint >/dev/null 2>&1 || echo "golangci-lint не установлен — пропускаю (необязателен)"

## build: собирает бинарник hey-duo в корень проекта.
build:
	$(GO) build -o $(BINARY) $(CMD)

## test: запускает unit- и integration-тесты.
test:
	$(GO) test ./...

## lint: запускает golangci-lint во всём проекте.
lint:
	golangci-lint run ./...

## clean: удаляет собранный бинарник и кэш сборки.
clean:
	rm -f $(BINARY)
	$(GO) clean

## dev: собирает и запускает приложение в режиме разработчика.
# По умолчанию показывает справку; аргументы передаются через ARGS,
# например: make dev ARGS="health --config arca.yaml"
ARGS ?= --help
dev: build
	./$(BINARY) $(ARGS)