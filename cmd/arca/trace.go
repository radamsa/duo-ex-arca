// Подкоманда trace: показать события трассировки задачи.
package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/radamsa/duo-ex-arca/internal/config"
)

// runTrace выполняет подкоманду trace <task-id>.
func runTrace(args []string, cfg config.Config) error {
	if len(args) != 1 {
		return fmt.Errorf("arca: trace требует ровно один аргумент — ID задачи")
	}
	taskID := args[0]

	a, err := buildApp(cfg)
	if err != nil {
		return err
	}
	defer a.db.Close()

	events, err := a.traces.ListByTask(context.Background(), taskID)
	if err != nil {
		return err
	}

	if len(events) == 0 {
		fmt.Printf("Задача %s: событий трассировки нет\n", taskID)
		return nil
	}

	first := events[0].Timestamp
	fmt.Printf("Задача: %s\n", taskID)
	for _, ev := range events {
		fmt.Printf("%s  %-22s", ev.Timestamp.Format(time.RFC3339), ev.Type)
		if ev.Participant != "" {
			fmt.Printf("  %-14s", ev.Participant)
		} else {
			fmt.Printf("  %-14s", "")
		}
		if ev.Duration > 0 {
			fmt.Printf("  %s", ev.Duration.Round(time.Millisecond))
		}
		fmt.Printf("  +%s", ev.Timestamp.Sub(first).Round(time.Millisecond))
		if len(ev.Metadata) > 0 {
			fmt.Printf("  %s", formatMetadata(ev.Metadata))
		}
		fmt.Println()
	}
	return nil
}

// formatMetadata печатает метаданные события в форме key=value.
func formatMetadata(metadata map[string]string) string {
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out string
	for _, k := range keys {
		out += fmt.Sprintf("[%s=%s] ", k, metadata[k])
	}
	return out
}