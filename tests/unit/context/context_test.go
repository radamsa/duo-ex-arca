// Тесты ContextBuilder: содержимое и детерминированность.
package context_test

import (
	"strings"
	"testing"

	"github.com/radamsa/duo-ex-arca/internal/context"
	"github.com/radamsa/duo-ex-arca/internal/domain"
)

func mustTask(t *testing.T) domain.Task {
	t.Helper()
	task, err := domain.NewTask("t-1", "Какую БД выбрать?", "Нужна СУБД для Go-проекта", []string{"только Go", "без C toolchain"}, domain.NORMAL)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

// TestBuildContainsTask — контекст содержит задачу и ограничения.
func TestBuildContainsTask(t *testing.T) {
	b := context.New()
	task := mustTask(t)

	got, err := b.Build(task)
	if err != nil {
		t.Fatalf("Build вернул ошибку: %v", err)
	}
	if !contains(got, task.Title) {
		t.Error("контекст не содержит заголовка задачи")
	}
	if !contains(got, task.Description) {
		t.Error("контекст не содержит описания задачи")
	}
	for _, c := range task.Constraints {
		if !contains(got, c) {
			t.Errorf("контекст не содержит ограничения %q", c)
		}
	}
}

// TestBuildDeterministic — одинаковые задачи дают одинаковый контекст.
func TestBuildDeterministic(t *testing.T) {
	b := context.New()
	task := mustTask(t)

	first, err := b.Build(task)
	if err != nil {
		t.Fatalf("Build вернул ошибку: %v", err)
	}
	second, err := b.Build(task)
	if err != nil {
		t.Fatalf("Build вернул ошибку: %v", err)
	}
	if first != second {
		t.Fatal("контекст должен быть детерминированным")
	}
}

// TestBuildNoForeignContent — контекст не содержит ничего, кроме задачи
// и ограничений (база для изоляции initial proposals, TASK-041:
// полный тест изоляции выполняется на уровне Debate Engine).
func TestBuildNoForeignContent(t *testing.T) {
	b := context.New()
	task := mustTask(t)

	got, err := b.Build(task)
	if err != nil {
		t.Fatalf("Build вернул ошибку: %v", err)
	}

	for _, forbidden := range []string{"participant", "предложение B", "proposal B", "ответ другого участника"} {
		if contains(got, forbidden) {
			t.Errorf("контекст не должен содержать %q", forbidden)
		}
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}