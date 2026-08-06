package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jrschumacher/wails-kit/llm"
)

// chunk renders one OpenAI streaming chunk as an SSE data frame.
func chunk(delta, finishReason string) string {
	fr := "null"
	if finishReason != "" {
		fr = `"` + finishReason + `"`
	}
	return "data: " + `{"id":"c1","object":"chat.completion.chunk","created":1,` +
		`"model":"gpt-4o","choices":[{"index":0,"delta":` + delta +
		`,"finish_reason":` + fr + `}]}` + "\n\n"
}

const sseDone = "data: [DONE]\n\n"

// textStream is a complete SSE body for a plain text response.
func textStream(text, finishReason string) string {
	textJSON, _ := json.Marshal(text)
	return chunk(`{"role":"assistant","content":""}`, "") +
		chunk(`{"content":`+string(textJSON)+`}`, "") +
		chunk(`{}`, finishReason) +
		sseDone
}

// toolCallStream is a complete SSE body for a tool-calling response, with the
// arguments split across chunks the way the real API sends them.
func toolCallStream(finishReason string) string {
	return chunk(`{"role":"assistant","content":null,"tool_calls":[`+
		`{"index":0,"id":"tu_1","type":"function",`+
		`"function":{"name":"get_weather","arguments":""}}]}`, "") +
		chunk(`{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}`, "") +
		chunk(`{"tool_calls":[{"index":0,"function":{"arguments":"\"NYC\"}"}}]}`, "") +
		chunk(`{}`, finishReason) +
		sseDone
}

// newSSEServer serves body as an event stream and records the request headers
// and decoded request body.
func newSSEServer(t *testing.T, body string) (*httptest.Server, *http.Header, *map[string]any) {
	t.Helper()
	var seenHeaders http.Header
	seenBody := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeaders = r.Header.Clone()
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &seenBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seenHeaders, &seenBody
}

func clearAuthEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OPENAI_API_KEY", "")
	_ = os.Unsetenv("OPENAI_API_KEY")
	t.Setenv("CF_AIG_AUTHORIZATION", "")
	_ = os.Unsetenv("CF_AIG_AUTHORIZATION")
}

func collect(t *testing.T, p *Provider, req llm.ChatRequest) []llm.StreamEvent {
	t.Helper()
	var events []llm.StreamEvent
	if err := p.StreamChat(context.Background(), req, func(e llm.StreamEvent) {
		events = append(events, e)
	}); err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	return events
}

func simpleReq() llm.ChatRequest {
	return llm.ChatRequest{Messages: []llm.ChatMessage{{Role: llm.RoleUser, Content: "hi"}}}
}

// --- Fix 3: a terminal event must be emitted for every finish reason. ---

func TestStreamChatTerminalEvent(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantStopReason llm.StopReason
	}{
		{"stop", textStream("hello", "stop"), llm.StopReasonEndTurn},
		// Regression: a "length" finish used to emit no terminal event at all,
		// hanging any consumer keyed on "done".
		{"length", textStream("hello", "length"), llm.StopReasonMaxTokens},
		{"content_filter", textStream("hello", "content_filter"), llm.StopReasonContentFilter},
		{"unknown reason", textStream("hello", "brand_new"), llm.StopReasonUnknown},
		{"absent finish reason", textStream("hello", ""), llm.StopReasonEndTurn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearAuthEnv(t)
			srv, _, _ := newSSEServer(t, tt.body)
			p := New("gpt-4o", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})
			events := collect(t, p, simpleReq())

			if len(events) == 0 {
				t.Fatal("no events emitted")
			}
			last := events[len(events)-1]
			if last.Type != llm.EventDone {
				t.Fatalf("last event type = %q, want %q", last.Type, llm.EventDone)
			}
			if last.StopReason != tt.wantStopReason {
				t.Errorf("StopReason = %q, want %q", last.StopReason, tt.wantStopReason)
			}

			var doneCount int
			var text strings.Builder
			for _, e := range events {
				if e.Type == llm.EventDone {
					doneCount++
				}
				if e.Type == llm.EventDelta {
					text.WriteString(e.Text)
				}
			}
			if doneCount != 1 {
				t.Errorf("emitted %d done events, want exactly 1", doneCount)
			}
			if text.String() != "hello" {
				t.Errorf("accumulated delta text = %q, want %q", text.String(), "hello")
			}
		})
	}
}

// --- Fix 2: tool calls must round-trip instead of being silently dropped. ---

func TestStreamChatEmitsToolUses(t *testing.T) {
	clearAuthEnv(t)
	srv, _, _ := newSSEServer(t, toolCallStream("tool_calls"))
	p := New("gpt-4o", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})
	events := collect(t, p, simpleReq())

	var toolUseEvents int
	for _, e := range events {
		if e.Type != llm.EventToolUse {
			continue
		}
		toolUseEvents++
		if len(e.ToolUses) != 1 {
			t.Fatalf("got %d tool uses, want 1", len(e.ToolUses))
		}
		tu := e.ToolUses[0]
		if tu.ID != "tu_1" {
			t.Errorf("tool use ID = %q, want tu_1", tu.ID)
		}
		if tu.Name != "get_weather" {
			t.Errorf("tool use Name = %q, want get_weather", tu.Name)
		}
		var input map[string]any
		if err := json.Unmarshal(tu.Input, &input); err != nil {
			t.Fatalf("tool arguments are not valid JSON (%s): %v", tu.Input, err)
		}
		if input["city"] != "NYC" {
			t.Errorf("tool input = %v, want city=NYC", input)
		}
	}
	if toolUseEvents != 1 {
		t.Fatalf("emitted %d tool_use events, want 1", toolUseEvents)
	}

	last := events[len(events)-1]
	if last.Type != llm.EventDone || last.StopReason != llm.StopReasonToolUse {
		t.Errorf("terminal event = %+v, want done/tool_use", last)
	}
}

// Even if the server reports "stop", the presence of tool calls is definitive.
func TestStreamChatToolUsesWithStopFinishReason(t *testing.T) {
	clearAuthEnv(t)
	srv, _, _ := newSSEServer(t, toolCallStream("stop"))
	p := New("gpt-4o", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})
	events := collect(t, p, simpleReq())

	last := events[len(events)-1]
	if last.StopReason != llm.StopReasonToolUse {
		t.Errorf("StopReason = %q, want %q", last.StopReason, llm.StopReasonToolUse)
	}
}

// The request must actually carry the tool definitions and the prior tool turns.
func TestStreamChatSendsToolsAndToolTurns(t *testing.T) {
	clearAuthEnv(t)
	srv, _, body := newSSEServer(t, textStream("ok", "stop"))
	p := New("gpt-4o", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})

	collect(t, p, llm.ChatRequest{
		SystemPrompt: "be helpful",
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: "weather?"},
			{
				Role:     llm.RoleAssistant,
				ToolUses: []llm.ToolUseBlock{{ID: "tu_1", Name: "w", Input: json.RawMessage(`{"c":"NYC"}`)}},
			},
			{Role: llm.RoleUser, ToolResults: []llm.ToolResult{{ToolUseID: "tu_1", Content: "sunny"}}},
		},
		Tools: []llm.ToolDefinition{{
			Name:        "w",
			Description: "weather",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"c": map[string]any{"type": "string"}},
				"required":   []string{"c"},
			},
		}},
	})

	raw, err := json.Marshal(*body)
	if err != nil {
		t.Fatalf("marshal captured body: %v", err)
	}
	got := string(raw)

	for _, want := range []string{
		`"tool_calls"`,
		`"tu_1"`,
		`"role":"tool"`,
		`"tool_call_id":"tu_1"`,
		`"tools"`,
		`"required":["c"]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("request body missing %s\nbody: %s", want, got)
		}
	}

	msgs, _ := (*body)["messages"].([]any)
	if len(msgs) != 4 {
		t.Errorf("sent %d messages, want 4 (system + user + assistant tool_calls + tool result)", len(msgs))
	}
}

func TestStreamChatRejectsUnsupportedRole(t *testing.T) {
	clearAuthEnv(t)
	srv, _, _ := newSSEServer(t, textStream("hi", "stop"))
	p := New("gpt-4o", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})

	var events []llm.StreamEvent
	err := p.StreamChat(context.Background(), llm.ChatRequest{
		Messages: []llm.ChatMessage{{Role: "developer", Content: "hi"}},
	}, func(e llm.StreamEvent) { events = append(events, e) })

	if err == nil {
		t.Fatal("expected an error for an unsupported role")
	}
	if len(events) != 1 || events[0].Type != llm.EventError {
		t.Fatalf("expected a single error event, got %+v", events)
	}
}

// --- Fix 1 parity: identical auth precedence, no environment mutation. ---

func TestNewDoesNotMutateEnvironment(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-key")
	t.Setenv("CF_AIG_AUTHORIZATION", "cf-token")

	_ = New("gpt-4o", llm.ProviderConfig{})

	if got := os.Getenv("OPENAI_API_KEY"); got != "env-key" {
		t.Fatalf("New mutated process environment: OPENAI_API_KEY = %q, want %q", got, "env-key")
	}
}

func TestAuthHeaders(t *testing.T) {
	tests := []struct {
		name      string
		envAPIKey string
		cfAuth    string
		configKey string
		wantAuth  string // "" means the header must be absent
		wantCF    string
	}{
		{
			name:      "configured key only",
			configKey: "cfg-key",
			wantAuth:  "Bearer cfg-key",
		},
		{
			name:      "configured key overrides environment",
			envAPIKey: "env-key",
			configKey: "cfg-key",
			wantAuth:  "Bearer cfg-key",
		},
		{
			name:      "environment fallback when nothing configured",
			envAPIKey: "env-key",
			wantAuth:  "Bearer env-key",
		},
		{
			name:      "gateway suppresses implicit environment key",
			envAPIKey: "env-key",
			cfAuth:    "cf-token",
			wantAuth:  "",
			wantCF:    "Bearer cf-token",
		},
		{
			name:      "gateway and configured key are both applied",
			envAPIKey: "env-key",
			cfAuth:    "cf-token",
			configKey: "cfg-key",
			wantAuth:  "Bearer cfg-key",
			wantCF:    "Bearer cf-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", tt.envAPIKey)
			if tt.envAPIKey == "" {
				_ = os.Unsetenv("OPENAI_API_KEY")
			}
			t.Setenv("CF_AIG_AUTHORIZATION", tt.cfAuth)
			if tt.cfAuth == "" {
				_ = os.Unsetenv("CF_AIG_AUTHORIZATION")
			}

			srv, headers, _ := newSSEServer(t, textStream("hi", "stop"))
			p := New("gpt-4o", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: tt.configKey})
			collect(t, p, simpleReq())

			if got := headers.Get("Authorization"); got != tt.wantAuth {
				t.Errorf("Authorization = %q, want %q", got, tt.wantAuth)
			}
			if _, present := (*headers)["Authorization"]; tt.wantAuth == "" && present {
				t.Errorf("Authorization header present but should be suppressed entirely")
			}
			if got := headers.Get("cf-aig-authorization"); got != tt.wantCF {
				t.Errorf("cf-aig-authorization = %q, want %q", got, tt.wantCF)
			}
		})
	}
}

// Parity with llm/anthropic: a cancelled context ends the stream as an error,
// with EventError and no EventDone.
func TestStreamChatContextCancellation(t *testing.T) {
	clearAuthEnv(t)
	srv, _, _ := newSSEServer(t, textStream("hello", "stop"))
	p := New("gpt-4o", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var events []llm.StreamEvent
	err := p.StreamChat(ctx, simpleReq(), func(e llm.StreamEvent) { events = append(events, e) })

	if err == nil {
		t.Fatal("StreamChat returned nil for a cancelled context")
	}
	for _, e := range events {
		if e.Type == llm.EventDone {
			t.Errorf("emitted a done event for a cancelled stream: %+v", e)
		}
	}
	if len(events) == 0 || events[len(events)-1].Type != llm.EventError {
		t.Fatalf("events = %+v, want a terminal error event", events)
	}
	if events[len(events)-1].Err == nil {
		t.Error("the error event carried no error")
	}
}

// A tool call whose arguments never arrive must still produce valid JSON.
// Passing an empty string through as Input would hand the consumer a
// ToolUseBlock that fails to unmarshal.
func TestStreamChatToolUseWithNoArguments(t *testing.T) {
	clearAuthEnv(t)
	body := chunk(`{"role":"assistant","content":null,"tool_calls":[`+
		`{"index":0,"id":"tu_1","type":"function","function":{"name":"ping","arguments":""}}]}`, "") +
		chunk(`{}`, "tool_calls") + sseDone

	srv, _, _ := newSSEServer(t, body)
	p := New("gpt-4o", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})
	events := collect(t, p, simpleReq())

	var found bool
	for _, e := range events {
		if e.Type != llm.EventToolUse {
			continue
		}
		found = true
		var input map[string]any
		if err := json.Unmarshal(e.ToolUses[0].Input, &input); err != nil {
			t.Fatalf("tool input %q is not valid JSON: %v", e.ToolUses[0].Input, err)
		}
		if len(input) != 0 {
			t.Errorf("tool input = %v, want an empty object", input)
		}
	}
	if !found {
		t.Fatalf("no tool_use event; events = %+v", events)
	}
}
