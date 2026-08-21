// Отображение активности в одну обновляемую строку: спиннер + стадии
// участников («participant-a: предлагает решение · participant-b: ...»).
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// spinnerFrames — кадры анимации (брайлевский спиннер).
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerInterval — период перерисовки строки активности.
const spinnerInterval = 100 * time.Millisecond

// activityReporter собирает текущие стадии участников и рисует их
// одной строкой, обновляемой на месте (через \r). Потокобезопасен:
// set вызывается из горутин движка, отрисовка идёт в своей горутине.
type activityReporter struct {
	mu     sync.Mutex
	stages map[string]string
	order  []string // стабильный порядок участников появления

	w   io.Writer
	tty bool // рисуем только в терминал; при перенаправлении вывода молчим

	stop    chan struct{}
	done    chan struct{}
	started bool
	stopped bool
	lastLen int
}

// newActivityReporter создаёт репортер. Рисование включается только
// если w — терминал.
func newActivityReporter(w io.Writer) *activityReporter {
	tty := false
	if f, ok := w.(*os.File); ok {
		if info, err := f.Stat(); err == nil {
			tty = info.Mode()&os.ModeCharDevice != 0
		}
	}
	return &activityReporter{
		stages: make(map[string]string),
		w:      w,
		tty:    tty,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// set фиксирует стадию участника (потокобезопасно, вызывается из движка).
// Нулевой репортер допустим (режим --dev) и ничего не делает.
func (r *activityReporter) set(participantID, stage string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, seen := r.stages[participantID]; !seen {
		r.order = append(r.order, participantID)
	}
	r.stages[participantID] = stage
}

// start запускает периодическую отрисовку (no-op вне терминала
// и для нулевого репортера).
func (r *activityReporter) start() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.tty || r.started {
		return
	}
	r.started = true
	go func() {
		defer close(r.done)
		ticker := time.NewTicker(spinnerInterval)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-r.stop:
				return
			case <-ticker.C:
				r.mu.Lock()
				line := r.renderLocked(spinnerFrames[i%len(spinnerFrames)])
				r.writeLocked(line)
				r.mu.Unlock()
			}
		}
	}()
}

// stopAndWait останавливает отрисовку, дожидается горутины и стирает
// строку активности (идемпотентно; no-op вне терминала и для нулевого
// репортера).
func (r *activityReporter) stopAndWait() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if !r.started || r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	close(r.stop)
	// Горутина берёт тот же мьютекс для отрисовки, поэтому ждём её
	// завершения после разблокировки.
	r.mu.Unlock()
	<-r.done

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastLen > 0 {
		fmt.Fprint(r.w, "\r"+strings.Repeat(" ", r.lastLen)+"\r")
		r.lastLen = 0
	}
}

// renderLocked собирает строку активности: «⠋ participant-a: предлагает
// решение · participant-b: критикует оппонента». Вызывать под мьютексом.
func (r *activityReporter) renderLocked(frame string) string {
	parts := make([]string, 0, len(r.order))
	for _, id := range r.order {
		parts = append(parts, id+": "+r.stages[id])
	}
	return frame + " " + strings.Join(parts, " · ")
}

// writeLocked пишет строку с \r и паддингом до длины предыдущей,
// чтобы стирать хвосты более длинных строк. Вызывать под мьютексом.
func (r *activityReporter) writeLocked(line string) {
	padding := r.lastLen - len([]rune(line))
	if padding < 0 {
		padding = 0
	}
	fmt.Fprint(r.w, "\r"+line+strings.Repeat(" ", padding))
	r.lastLen = len([]rune(line))
}
