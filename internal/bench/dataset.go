// Пакет bench — бенчмарк-харнесс (docs/plan-mvp.md, TASK-150..153).
//
// Запускает датасет задач (JSONL) через агента в заданном режиме
// (baseline FAST или duo-дебат) и оценивает результаты детерминированным
// эвалуатором: точное совпадение нормализованного решения с ожиданием.
package bench

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// DatasetItem — одна задача датасета в формате JSONL (TASK-150).
type DatasetItem struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Task     string `json:"task"`
	Expected string `json:"expected"`
}

// LoadDataset читает JSONL-датасет: по одной задаче на строку.
// Пустые строки пропускаются, обязательны id и task.
func LoadDataset(path string) ([]DatasetItem, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("bench: открытие датасета: %w", err)
	}
	defer file.Close()

	var items []DatasetItem
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var item DatasetItem
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("bench: строка %d: невалидный JSON: %w", lineNumber, err)
		}
		if item.ID == "" {
			return nil, fmt.Errorf("bench: строка %d: пустой id", lineNumber)
		}
		if item.Task == "" {
			return nil, fmt.Errorf("bench: строка %d: пустой task", lineNumber)
		}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("bench: чтение датасета: %w", err)
	}
	return items, nil
}