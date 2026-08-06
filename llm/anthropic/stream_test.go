package anthropic

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

// sseEvent renders one Anthropic server-sent event.
func sseEvent(name, data string) string {
	return "event: " + name + "\ndata: " + data + "\n\n"
}

const (
	evtMessageStart = `{"type":"message_start","message":{"id":"msg_01","type":"message",` +
		`"role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":null,` +
		`"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`
	evtTextBlockStart = `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
	evtBlockStop      = `{"type":"content_block_stop","index":0}`
	evtMessageStop    = `{"type":"message_stop"}`
)

func textDelta(text string) string {
	return `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + text + `"}}`
}

func messageDelta(stopReason string) string {
	return `{"type":"message_delta","delta":{"stop_reason":"` + stopReason +
		`","stop_sequence":null},"usage":{"output_tokens":5}}`
}

// textStream is a complete SSE body for a plain text response.
func textStream(text, stopReason string) string {
	return sseEvent("message_start", evtMessageStart) +
		sseEvent("content_block_start", evtTextBlockStart) +
		sseEvent("content_block_delta", textDelta(text)) +
		sseEvent("content_block_stop", evtBlockStop) +
		sseEvent("message_delta", messageDelta(stopReason)) +
		sseEvent("message_stop", evtMessageStop)
}

// toolUseStream is a complete SSE body for a tool-calling response.
func toolUseStream() string {
	blockStart := `{"type":"content_block_start","index":0,"content_block":` +
		`{"type":"tool_use","id":"tu_1","name":"get_weather","input":{}}}`
	inputDelta := `{"type":"content_block_delta","index":0,"delta":` +
		`{"type":"input_json_delta","partial_json":"{\"city\":\"NYC\"}"}}`
	return sseEvent("message_start", evtMessageStart) +
		sseEvent("content_block_start", blockStart) +
		sseEvent("content_block_delta", inputDelta) +
		sseEvent("content_block_stop", evtBlockStop) +
		sseEvent("message_delta", messageDelta("tool_use")) +
		sseEvent("message_stop", evtMessageStop)
}

// newSSEServer serves body as an event stream and records the last request headers.
func newSSEServer(t *testing.T, body string) (*httptest.Server, *http.Header) {
	t.Helper()
	srv, headers, _ := newRecordingSSEServer(t, body)
	return srv, headers
}

// newRecordingSSEServer also decodes the last request body, for tests that
// assert on what was actually sent.
func newRecordingSSEServer(t *testing.T, body string) (*httptest.Server, *http.Header, *map[string]any) {
	t.Helper()
	var seen http.Header
	seenBody := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
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
	return srv, &seen, &seenBody
}

func collect(t *testing.T, p *Provider) []llm.StreamEvent {
	t.Helper()
	var events []llm.StreamEvent
	err := p.StreamChat(context.Background(), llm.ChatRequest{
		Messages: []llm.ChatMessage{{Role: llm.RoleUser, Content: "hi"}},
	}, func(e llm.StreamEvent) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	return events
}

// --- Fix 1: auth options must not mutate the process environment, and a
// configured API key must not be discarded when a Cloudflare gateway token is
// present. ---

func TestNewDoesNotMutateEnvironment(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-key")
	t.Setenv("CF_AIG_AUTHORIZATION", "cf-token")

	_ = New("m", llm.ProviderConfig{})

	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "env-key" {
		t.Fatalf("New mutated process environment: ANTHROPIC_API_KEY = %q, want %q", got, "env-key")
	}
}

func TestAuthHeaders(t *testing.T) {
	tests := []struct {
		name       string
		envAPIKey  string
		cfAuth     string
		configKey  string
		wantAPIKey string // "" means the header must be absent
		wantCF     string
	}{
		{
			name:       "configured key only",
			configKey:  "cfg-key",
			wantAPIKey: "cfg-key",
		},
		{
			name:       "configured key overrides environment",
			envAPIKey:  "env-key",
			configKey:  "cfg-key",
			wantAPIKey: "cfg-key",
		},
		{
			name:       "environment fallback when nothing configured",
			envAPIKey:  "env-key",
			wantAPIKey: "env-key",
		},
		{
			// The gateway holds the provider credential, so the implicit
			// environment-derived key must be suppressed - without unsetting it.
			name:       "gateway suppresses implicit environment key",
			envAPIKey:  "env-key",
			cfAuth:     "cf-token",
			wantAPIKey: "",
			wantCF:     "Bearer cf-token",
		},
		{
			// Regression: the configured key used to be discarded entirely
			// whenever CF_AIG_AUTHORIZATION was set.
			name:       "gateway and configured key are both applied",
			envAPIKey:  "env-key",
			cfAuth:     "cf-token",
			configKey:  "cfg-key",
			wantAPIKey: "cfg-key",
			wantCF:     "Bearer cf-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANTHROPIC_API_KEY", tt.envAPIKey)
			if tt.envAPIKey == "" {
				_ = os.Unsetenv("ANTHROPIC_API_KEY")
			}
			t.Setenv("CF_AIG_AUTHORIZATION", tt.cfAuth)
			if tt.cfAuth == "" {
				_ = os.Unsetenv("CF_AIG_AUTHORIZATION")
			}
			// Keep the SDK's other implicit sources out of the picture.
			t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
			_ = os.Unsetenv("ANTHROPIC_AUTH_TOKEN")

			srv, headers := newSSEServer(t, textStream("hi", "end_turn"))
			p := New("claude-sonnet-4-6", llm.ProviderConfig{
				BaseURL: srv.URL + "/",
				APIKey:  tt.configKey,
			})
			collect(t, p)

			if got := headers.Get("X-Api-Key"); got != tt.wantAPIKey {
				t.Errorf("X-Api-Key = %q, want %q", got, tt.wantAPIKey)
			}
			if _, present := (*headers)["X-Api-Key"]; tt.wantAPIKey == "" && present {
				t.Errorf("X-Api-Key header present but should be suppressed entirely")
			}
			if got := headers.Get("cf-aig-authorization"); got != tt.wantCF {
				t.Errorf("cf-aig-authorization = %q, want %q", got, tt.wantCF)
			}
		})
	}
}

// --- Fix 3 (parity): a terminal event is emitted for every finish reason. ---

func TestStreamChatTerminalEvent(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantStopReason llm.StopReason
		wantToolUse    bool
	}{
		{"end_turn", textStream("hello", "end_turn"), llm.StopReasonEndTurn, false},
		{"max_tokens", textStream("hello", "max_tokens"), llm.StopReasonMaxTokens, false},
		{"stop_sequence", textStream("hello", "stop_sequence"), llm.StopReasonStopSequence, false},
		{"refusal", textStream("hello", "refusal"), llm.StopReasonContentFilter, false},
		{"tool_use", toolUseStream(), llm.StopReasonToolUse, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CF_AIG_AUTHORIZATION", "")
			_ = os.Unsetenv("CF_AIG_AUTHORIZATION")

			srv, _ := newSSEServer(t, tt.body)
			p := New("claude-sonnet-4-6", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})
			events := collect(t, p)

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

			var doneCount, toolUseCount int
			for _, e := range events {
				switch e.Type {
				case llm.EventDone:
					doneCount++
				case llm.EventToolUse:
					toolUseCount++
					if len(e.ToolUses) == 0 {
						t.Error("tool_use event carried no tool uses")
					}
					if e.ToolUses[0].Name != "get_weather" {
						t.Errorf("tool name = %q, want get_weather", e.ToolUses[0].Name)
					}
					if !strings.Contains(string(e.ToolUses[0].Input), "NYC") {
						t.Errorf("tool input = %s, want it to contain NYC", e.ToolUses[0].Input)
					}
				}
			}
			if doneCount != 1 {
				t.Errorf("emitted %d done events, want exactly 1", doneCount)
			}
			if tt.wantToolUse && toolUseCount != 1 {
				t.Errorf("emitted %d tool_use events, want 1", toolUseCount)
			}
		})
	}
}

func TestStreamChatEmitsTextDeltas(t *testing.T) {
	t.Setenv("CF_AIG_AUTHORIZATION", "")
	_ = os.Unsetenv("CF_AIG_AUTHORIZATION")

	srv, _ := newSSEServer(t, textStream("hello", "end_turn"))
	p := New("claude-sonnet-4-6", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})
	events := collect(t, p)

	var text strings.Builder
	for _, e := range events {
		if e.Type == llm.EventDelta {
			text.WriteString(e.Text)
		}
	}
	if text.String() != "hello" {
		t.Errorf("accumulated delta text = %q, want %q", text.String(), "hello")
	}
}

func TestStreamChatRejectsUnsupportedRole(t *testing.T) {
	t.Setenv("CF_AIG_AUTHORIZATION", "")
	_ = os.Unsetenv("CF_AIG_AUTHORIZATION")

	srv, _ := newSSEServer(t, textStream("hi", "end_turn"))
	p := New("claude-sonnet-4-6", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})

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

// The Provider contract says a cancelled context ends the stream as an error,
// with EventError and no EventDone. A consumer that renders "done" on the
// terminal event must not be told a cancelled request completed normally.
func TestStreamChatContextCancellation(t *testing.T) {
	t.Setenv("CF_AIG_AUTHORIZATION", "")
	_ = os.Unsetenv("CF_AIG_AUTHORIZATION")

	srv, _ := newSSEServer(t, textStream("hello", "end_turn"))
	p := New("claude-sonnet-4-6", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var events []llm.StreamEvent
	err := p.StreamChat(ctx, llm.ChatRequest{
		Messages: []llm.ChatMessage{{Role: llm.RoleUser, Content: "hi"}},
	}, func(e llm.StreamEvent) { events = append(events, e) })

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
