package llm

import "sync"

// ModelBudget describes a model's token limits.
type ModelBudget struct {
	ContextWindow   int // total context window size in tokens
	DefaultMaxReply int // default max tokens for replies (0 = use ContextBuilder default)
}

var (
	budgetMu sync.RWMutex
	budgets  = map[string]ModelBudget{}
)

// RegisterModelBudget registers the token budget for a model ID.
func RegisterModelBudget(modelID string, budget ModelBudget) {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	budgets[modelID] = budget
}

// GetModelBudget returns the budget for a model, or a zero value if not registered.
func GetModelBudget(modelID string) (ModelBudget, bool) {
	budgetMu.RLock()
	defer budgetMu.RUnlock()
	b, ok := budgets[modelID]
	return b, ok
}

func init() {
	// Anthropic models
	RegisterModelBudget("claude-opus-4-6", ModelBudget{ContextWindow: 200000, DefaultMaxReply: 16384})
	RegisterModelBudget("claude-sonnet-4-6", ModelBudget{ContextWindow: 200000, DefaultMaxReply: 16384})
	RegisterModelBudget("claude-haiku-4-5-20251001", ModelBudget{ContextWindow: 200000, DefaultMaxReply: 8192})

	// OpenAI models
	RegisterModelBudget("gpt-4o", ModelBudget{ContextWindow: 128000, DefaultMaxReply: 16384})
	RegisterModelBudget("gpt-4o-mini", ModelBudget{ContextWindow: 128000, DefaultMaxReply: 16384})
	RegisterModelBudget("o3", ModelBudget{ContextWindow: 200000, DefaultMaxReply: 100000})
}
