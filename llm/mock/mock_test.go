package mock

import (
	"context"
	"errors"
	"testing"

	"github.com/jrschumacher/wails-kit/llm"
)

// The mock must satisfy the Provider interface.
var _ llm.Provider = (*Provider)(nil)

func TestProviderIdentity(t *testing.T) {
	p := &Provider{Name: "test", Model: "test-model"}
	if got := p.ProviderName(); got != "test" {
		t.Errorf("ProviderName() = %q, want %q", got, "test")
	}
	if got := p.ModelID(); got != "test-model" {
		t.Errorf("ModelID() = %q, want %q", got, "test-model")
	}
}

func TestDefaultStreamChatEmitsDeltaThenDone(t *testing.T) {
	p := &Provider{Name: "test", Model: "test-model"}

	var events []llm.StreamEvent
	if err := p.StreamChat(context.Background(), llm.ChatRequest{}, func(e llm.StreamEvent) {
		events = append(events, e)
	}); err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(events), events)
	}
	if events[0].Type != llm.EventDelta || events[0].Text != "mock response" {
		t.Errorf("first event = %+v, want a delta with %q", events[0], "mock response")
	}
	if events[1].Type != llm.EventDone {
		t.Errorf("last event type = %q, want %q", events[1].Type, llm.EventDone)
	}
	if events[1].StopReason != llm.StopReasonEndTurn {
		t.Errorf("StopReason = %q, want %q", events[1].StopReason, llm.StopReasonEndTurn)
	}
}

func TestOnStreamChatOverrideReceivesRequest(t *testing.T) {
	wantErr := errors.New("boom")
	var gotReq llm.ChatRequest

	p := &Provider{
		Name: "test",
		OnStreamChat: func(_ context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error {
			gotReq = req
			handler(llm.StreamEvent{Type: llm.EventDelta, Text: "custom"})
			return wantErr
		},
	}

	var events []llm.StreamEvent
	req := llm.ChatRequest{
		SystemPrompt: "sys",
		Messages:     []llm.ChatMessage{{Role: llm.RoleUser, Content: "hi"}},
		Tools:        []llm.ToolDefinition{{Name: "t"}},
	}
	err := p.StreamChat(context.Background(), req, func(e llm.StreamEvent) {
		events = append(events, e)
	})

	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	if len(events) != 1 || events[0].Text != "custom" {
		t.Errorf("events = %+v, want a single custom delta", events)
	}
	if gotReq.SystemPrompt != "sys" || len(gotReq.Messages) != 1 || len(gotReq.Tools) != 1 {
		t.Errorf("override received %+v, want the request passed through unchanged", gotReq)
	}
}

// The mock's registration-free construction must keep working with untyped
// string literals, which is how the README documents it.
func TestUntypedLiteralsStillWork(t *testing.T) {
	p := &Provider{
		Name: "test",
		OnStreamChat: func(_ context.Context, _ llm.ChatRequest, handler func(llm.StreamEvent)) error {
			handler(llm.StreamEvent{Type: "delta", Text: "test response"})
			handler(llm.StreamEvent{Type: "done", StopReason: "end_turn"})
			return nil
		},
	}
	var events []llm.StreamEvent
	if err := p.StreamChat(context.Background(), llm.ChatRequest{}, func(e llm.StreamEvent) {
		events = append(events, e)
	}); err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) != 2 || events[1].Type != llm.EventDone {
		t.Fatalf("events = %+v", events)
	}
}
