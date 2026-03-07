package keyring

import (
	"testing"
)

func TestMemoryStore(t *testing.T) {
	s := NewMemoryStore()

	// Get missing key
	_, err := s.Get("missing")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if s.Has("missing") {
		t.Fatal("Has should return false for missing key")
	}

	// Set and get
	if err := s.Set("key1", "value1"); err != nil {
		t.Fatal(err)
	}
	val, err := s.Get("key1")
	if err != nil {
		t.Fatal(err)
	}
	if val != "value1" {
		t.Fatalf("expected value1, got %s", val)
	}
	if !s.Has("key1") {
		t.Fatal("Has should return true")
	}

	// Overwrite
	if err := s.Set("key1", "value2"); err != nil {
		t.Fatal(err)
	}
	val, _ = s.Get("key1")
	if val != "value2" {
		t.Fatalf("expected value2, got %s", val)
	}

	// Delete
	if err := s.Delete("key1"); err != nil {
		t.Fatal(err)
	}
	if s.Has("key1") {
		t.Fatal("Has should return false after delete")
	}

	// Delete non-existent is not an error
	if err := s.Delete("nope"); err != nil {
		t.Fatal(err)
	}
}

func TestSetGetJSON(t *testing.T) {
	s := NewMemoryStore()

	type creds struct {
		User  string `json:"user"`
		Token string `json:"token"`
	}

	input := creds{User: "alice", Token: "abc123"}
	if err := SetJSON(s, "creds", input); err != nil {
		t.Fatal(err)
	}

	var output creds
	if err := GetJSON(s, "creds", &output); err != nil {
		t.Fatal(err)
	}
	if output != input {
		t.Fatalf("expected %+v, got %+v", input, output)
	}

	// Missing key
	if err := GetJSON(s, "missing", &output); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestEnvKey(t *testing.T) {
	tests := []struct {
		prefix, key, want string
	}{
		{"MYAPP", "llm.anthropic.secret", "MYAPP_LLM_ANTHROPIC_SECRET"},
		{"APP", "api-key", "APP_API_KEY"},
		{"X", "simple", "X_SIMPLE"},
	}
	for _, tt := range tests {
		got := EnvKey(tt.prefix, tt.key)
		if got != tt.want {
			t.Errorf("EnvKey(%q, %q) = %q, want %q", tt.prefix, tt.key, got, tt.want)
		}
	}
}

func TestEnvFallback(t *testing.T) {
	t.Setenv("TEST_MY_KEY", "envvalue")

	val, ok := envFallback("TEST", "my.key")
	if !ok || val != "envvalue" {
		t.Fatalf("expected envvalue, got %q (ok=%v)", val, ok)
	}

	val, ok = envFallback("TEST", "missing")
	if ok {
		t.Fatalf("expected no fallback, got %q", val)
	}

	// Empty prefix disables fallback
	val, ok = envFallback("", "my.key")
	if ok {
		t.Fatalf("expected no fallback with empty prefix, got %q", val)
	}
}
