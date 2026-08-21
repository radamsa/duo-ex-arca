// Декоратор DebugClient — режим разработчика: полные запросы и ответы LLM
// пишутся в указанный writer (обычно stderr).
package llm

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// DebugClient оборачивает LLM и выводит каждое обращение: промпт целиком,
// полный ответ модели (или ошибку), длительность и расход токенов.
type DebugClient struct {
	label string
	w     io.Writer
	inner LLM
}

// DebugClient создаёт отладочную обёртку над inner.
// label — имя участника для различения A и B в логе.
func NewDebugClient(label string, w io.Writer, inner LLM) *DebugClient {
	return &DebugClient{label: label, w: w, inner: inner}
}

// Generate выполняет запрос и пишет детали в writer.
func (d *DebugClient) Generate(ctx context.Context, request GenerationRequest) (GenerationResponse, error) {
	d.header("запрос")
	for _, m := range request.Messages {
		fmt.Fprintf(d.w, "[%s]\n%s\n", strings.ToUpper(string(m.Role)), m.Content)
	}

	start := time.Now()
	response, err := d.inner.Generate(ctx, request)
	elapsed := time.Since(start)

	if err != nil {
		d.header(fmt.Sprintf("ошибка (%s)", elapsed))
		fmt.Fprintf(d.w, "%v\n", err)
		return response, err
	}

	d.header(fmt.Sprintf("ответ (%s, %d токенов)", elapsed, response.Usage.TotalTokens))
	fmt.Fprintf(d.w, "%s\n", response.Content)
	return response, err
}

// header печатает разделитель с меткой времени и именем участника.
func (d *DebugClient) header(what string) {
	fmt.Fprintf(d.w, "────── %s %s: %s ──────\n", time.Now().Format("15:04:05.000"), d.label, what)
}
