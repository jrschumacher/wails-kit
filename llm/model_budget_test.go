package llm

import "testing"

func TestGetModelBudget_RegisteredModels(t *testing.T) {
	tests := []struct {
		modelID       string
		wantWindow    int
		wantMaxReply  int
	}{
		{"claude-sonnet-4-6", 200000, 16384},
		{"claude-opus-4-6", 200000, 16384},
		{"claude-haiku-4-5-20251001", 200000, 8192},
		{"gpt-4o", 128000, 16384},
		{"gpt-4o-mini", 128000, 16384},
		{"o3", 200000, 100000},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			budget, ok := GetModelBudget(tt.modelID)
			if !ok {
				t.Fatalf("model %q not registered", tt.modelID)
			}
			if budget.ContextWindow != tt.wantWindow {
				t.Errorf("context window: got %d, want %d", budget.ContextWindow, tt.wantWindow)
			}
			if budget.DefaultMaxReply != tt.wantMaxReply {
				t.Errorf("default max reply: got %d, want %d", budget.DefaultMaxReply, tt.wantMaxReply)
			}
		})
	}
}

func TestGetModelBudget_UnknownModel(t *testing.T) {
	_, ok := GetModelBudget("nonexistent-model")
	if ok {
		t.Error("expected unknown model to return false")
	}
}

func TestRegisterModelBudget_Custom(t *testing.T) {
	RegisterModelBudget("test-model-xyz", ModelBudget{ContextWindow: 32000, DefaultMaxReply: 2048})
	budget, ok := GetModelBudget("test-model-xyz")
	if !ok {
		t.Fatal("expected registered model to be found")
	}
	if budget.ContextWindow != 32000 {
		t.Errorf("context window: got %d, want 32000", budget.ContextWindow)
	}
	if budget.DefaultMaxReply != 2048 {
		t.Errorf("default max reply: got %d, want 2048", budget.DefaultMaxReply)
	}
}
