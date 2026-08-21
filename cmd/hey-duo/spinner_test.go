// Тесты репортера активности: рендер строки, nil-безопасность, гонки.
package main

import (
	"os"
	"strings"
	"sync"
	"testing"
)

// TestActivityReporterNilSafe — нулевой репортер (--dev) не паникует.
func TestActivityReporterNilSafe(t *testing.T) {
	var r *activityReporter
	r.set("participant-a", "предлагает решение")
	r.start()
	r.stopAndWait()
}

// TestActivityReporterRender — строка содержит всех участников и стадии.
func TestActivityReporterRender(t *testing.T) {
	r := newActivityReporter(os.Stdout)
	r.set("participant-a", "предлагает решение")
	r.set("participant-b", "критикует оппонента")

	line := func() string {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.renderLocked("⠋")
	}()

	for _, want := range []string{"⠋", "participant-a: предлагает решение", "participant-b: критикует оппонента"} {
		if !strings.Contains(line, want) {
			t.Fatalf("в строке %q нет %q", line, want)
		}
	}
}

// TestActivityReporterUpdateStage — стадия участника обновляется на месте,
// порядок участников стабильный.
func TestActivityReporterUpdateStage(t *testing.T) {
	r := newActivityReporter(os.Stdout)
	r.set("participant-b", "предлагает решение")
	r.set("participant-a", "предлагает решение")
	r.set("participant-b", "оценивает консенсус")

	line := func() string {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.renderLocked("|")
	}()

	// Порядок участников — порядок первого появления; стадия обновляется на месте.
	want := "| participant-b: оценивает консенсус · participant-a: предлагает решение"
	if line != want {
		t.Fatalf("render = %q, ожидалось %q", line, want)
	}
}

// TestActivityReporterConcurrentSet — set из нескольких горутин безопасен
// (запускать с -race).
func TestActivityReporterConcurrentSet(t *testing.T) {
	r := newActivityReporter(os.Stdout)

	var wg sync.WaitGroup
	stages := []string{"предлагает решение", "критикует оппонента", "оценивает консенсус"}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "participant-" + string(rune('a'+i%2))
			for _, s := range stages {
				r.set(id, s)
			}
		}(i)
	}
	wg.Wait()

	line := func() string {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.renderLocked("|")
	}()
	if !strings.Contains(line, "participant-a") || !strings.Contains(line, "participant-b") {
		t.Fatalf("строка потеряла участников: %q", line)
	}
}

// TestActivityReporterSilentWhenNotTTY — при перенаправлении вывода
// start/stop ничего не пишут.
func TestActivityReporterSilentWhenNotTTY(t *testing.T) {
	var buf syncBuffer
	r := newActivityReporter(&buf)
	r.set("participant-a", "предлагает решение")
	r.start()
	r.stopAndWait()
	if buf.Len() != 0 {
		t.Fatalf("не-TTY вывод должен быть пустым, получено %q", buf.String())
	}
}

// syncBuffer — потокобезопасная заглушка вывода.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
