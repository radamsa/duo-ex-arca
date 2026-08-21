// Тесты разбора глобальных флагов CLI.
package main

import (
	"reflect"
	"testing"
)

// TestExtractFlagsLog --log разбирается в обеих формах и сочетается
// с другими флагами.
func TestExtractFlagsLog(t *testing.T) {
	path, dev, logPath, rest, err := extractFlags([]string{
		"--config", "arca.yaml", "--dev", "--log", "debate.log", "--mode", "fast", "вопрос",
	})
	if err != nil {
		t.Fatalf("extractFlags вернул ошибку: %v", err)
	}
	if path != "arca.yaml" || !dev || logPath != "debate.log" {
		t.Fatalf("path=%q dev=%v logPath=%q", path, dev, logPath)
	}
	if !reflect.DeepEqual(rest, []string{"--mode", "fast", "вопрос"}) {
		t.Fatalf("rest = %v", rest)
	}
}

// TestExtractFlagsLogEquals --log=<файл>.
func TestExtractFlagsLogEquals(t *testing.T) {
	_, _, logPath, rest, err := extractFlags([]string{"--log=/tmp/debate.log", "вопрос"})
	if err != nil {
		t.Fatalf("extractFlags вернул ошибку: %v", err)
	}
	if logPath != "/tmp/debate.log" {
		t.Fatalf("logPath = %q", logPath)
	}
	if !reflect.DeepEqual(rest, []string{"вопрос"}) {
		t.Fatalf("rest = %v", rest)
	}
}

// TestExtractFlagsLogMissingValue --log без значения — ошибка.
func TestExtractFlagsLogMissingValue(t *testing.T) {
	if _, _, _, _, err := extractFlags([]string{"--log"}); err == nil {
		t.Fatal("ожидалась ошибка: --log без значения")
	}
}

// TestExtractFlagsNoLog — без флага лог пуст.
func TestExtractFlagsNoLog(t *testing.T) {
	_, dev, logPath, _, err := extractFlags([]string{"вопрос"})
	if err != nil {
		t.Fatalf("extractFlags вернул ошибку: %v", err)
	}
	if dev || logPath != "" {
		t.Fatalf("dev=%v logPath=%q, ожидались нулевые значения", dev, logPath)
	}
}
