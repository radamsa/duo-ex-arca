// Команда hey-duo — CLI для Duo ex Arca (docs/plan-mvp.md, TASK-100..104).
//
// Подкоманды:
//
//	hey-duo ask "вопрос"                 — задать вопрос агенту
//	hey-duo ask --mode fast "вопрос"     — задать режим
//	hey-duo ask --json "вопрос"          — вывод решения в JSON
//	hey-duo trace <task-id>              — показать trace задачи
//	hey-duo config                       — показать конфигурацию
//	hey-duo health                       — проверить доступность LLM
//
// Путь к конфигурации — флаг --config или переменная окружения ARCA_CONFIG;
// по умолчанию ./arca.yaml.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/radamsa/duo-ex-arca/internal/config"
)

const defaultConfigPath = "./arca.yaml"

// main разбирает подкоманду и выполняет её.
func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage()
		return
	}

	command := args[0]
	rest := args[1:]

	// Флаг --config может стоять после подкоманды: hey-duo ask --config path "..."
	cfgPath, rest, err := extractConfigFlag(rest)
	if err != nil {
		fail(err)
	}

	switch command {
	case "ask":
		fail(runAsk(rest, loadConfig(cfgPath)))
	case "trace":
		fail(runTrace(rest, loadConfig(cfgPath)))
	case "config":
		fail(runConfigView(rest, loadConfig(cfgPath)))
	case "health":
		fail(runHealth(rest, loadConfig(cfgPath)))
	case "bench":
		fail(runBench(rest, loadConfig(cfgPath)))
	case "setup":
		// setup не читает существующий конфиг — он его создаёт.
		fail(runSetup(rest, cfgPath))
	default:
		fmt.Fprintf(os.Stderr, "hey-duo: неизвестная команда %q\n", command)
		usage()
		os.Exit(2)
	}
}

// loadConfig читает конфигурацию по пути из флага, ARCA_CONFIG или по умолчанию.
func loadConfig(path string) config.Config {
	if path == "" {
		path = os.Getenv("ARCA_CONFIG")
	}
	if path == "" {
		path = defaultConfigPath
	}
	cfg, err := config.Load(path)
	if err != nil {
		fail(err)
	}
	return cfg
}

// extractConfigFlag вынимает --config <путь> из аргументов подкоманды.
func extractConfigFlag(args []string) (path string, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--config" || a == "-config":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("hey-duo: флаг %s требует путь к файлу", a)
			}
			path = args[i+1]
			i++
		case strings.HasPrefix(a, "--config="):
			path = strings.TrimPrefix(a, "--config=")
		case strings.HasPrefix(a, "-config="):
			path = strings.TrimPrefix(a, "-config=")
		default:
			rest = append(rest, a)
		}
	}
	return path, rest, nil
}

// usage печатает справку.
func usage() {
	fmt.Println("hey-duo — Duo ex Arca: агент с двумя независимыми LLM.")
	fmt.Println()
	fmt.Println("Использование:")
	fmt.Println("  hey-duo setup [--config <путь>]           интерактивная настройка")
	fmt.Println("  hey-duo ask --mode <mode> --json \"<вопрос>\"   задать вопрос агенту")
	fmt.Println("  hey-duo trace <task-id>                         показать trace задачи")
	fmt.Println("  hey-duo config                                 показать конфигурацию")
	fmt.Println("  hey-duo health --config <путь>                 проверить доступность LLM")
	fmt.Println("  hey-duo bench --dataset <file.jsonl> [--mode]  прогон бенчмарка")
	fmt.Println()
	fmt.Println("Режимы: fast, normal, deliberate, critical")
	fmt.Println("Конфигурация: --config <путь> или $ARCA_CONFIG (по умолчанию ./arca.yaml)")
}

// fail печатает ошибку и завершает процесс с кодом 1.
func fail(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "hey-duo:", err)
	os.Exit(1)
}