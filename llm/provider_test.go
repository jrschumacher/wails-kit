package llm

import "testing"

// The protocol constants must keep string as their underlying type so that
// existing consumers writing untyped literals (Role: "user", Type: "delta")
// keep compiling.
func TestProtocolConstantValues(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"RoleUser", string(RoleUser), "user"},
		{"RoleAssistant", string(RoleAssistant), "assistant"},
		{"RoleSystem", string(RoleSystem), "system"},
		{"EventDelta", string(EventDelta), "delta"},
		{"EventDone", string(EventDone), "done"},
		{"EventError", string(EventError), "error"},
		{"EventToolUse", string(EventToolUse), "tool_use"},
		{"StopReasonEndTurn", string(StopReasonEndTurn), "end_turn"},
		{"StopReasonToolUse", string(StopReasonToolUse), "tool_use"},
		{"StopReasonMaxTokens", string(StopReasonMaxTokens), "max_tokens"},
		{"StopReasonContentFilter", string(StopReasonContentFilter), "content_filter"},
		{"StopReasonStopSequence", string(StopReasonStopSequence), "stop_sequence"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

// Untyped string literals must still be assignable to the named-type fields,
// otherwise the change is breaking for existing consumers.
func TestUntypedLiteralsStillAssignable(t *testing.T) {
	m := ChatMessage{Role: "user", Content: "hi"}
	if m.Role != RoleUser {
		t.Errorf("literal %q did not match RoleUser", m.Role)
	}

	e := StreamEvent{Type: "done", StopReason: "end_turn"}
	if e.Type != EventDone {
		t.Errorf("literal %q did not match EventDone", e.Type)
	}
	if e.StopReason != StopReasonEndTurn {
		t.Errorf("literal %q did not match StopReasonEndTurn", e.StopReason)
	}

	// switch on a literal still compiles and matches
	switch e.Type {
	case "done":
	default:
		t.Errorf("switch on string literal did not match")
	}
}
