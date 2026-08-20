package llm

// Role — роль сообщения в диалоге.
type Role string

const (
	// RoleSystem — системный промпт.
	RoleSystem Role = "system"
	// RoleUser — сообщение пользователя.
	RoleUser Role = "user"
	// RoleAssistant — ответ модели.
	RoleAssistant Role = "assistant"
)

// Message — одно сообщение диалога.
type Message struct {
	Role    Role
	Content string
}

// GenerationRequest — запрос на генерацию.
type GenerationRequest struct {
	Model    string
	Messages []Message

	// Temperature — опциональная температура выборки.
	Temperature *float64
	// MaxTokens — опциональное ограничение длины ответа.
	MaxTokens *int
}

// Usage — статистика потребления токенов.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// GenerationResponse — результат генерации.
type GenerationResponse struct {
	Content string

	// FinishReason — причина завершения генерации ("stop", "length" и т.п.).
	FinishReason string

	Usage Usage
}