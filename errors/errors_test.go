package errors

import (
	stderrors "errors"
	"testing"
)

func TestNew(t *testing.T) {
	err := New(ErrAuthInvalid, "bad token", nil)
	if err.Code != ErrAuthInvalid {
		t.Errorf("expected code %s, got %s", ErrAuthInvalid, err.Code)
	}
	if err.Message != "bad token" {
		t.Errorf("expected message 'bad token', got %s", err.Message)
	}
	if err.UserMsg != defaultMessages[ErrAuthInvalid] {
		t.Errorf("expected user message %q, got %q", defaultMessages[ErrAuthInvalid], err.UserMsg)
	}
	if err.Error() != "bad token" {
		t.Errorf("expected Error() = 'bad token', got %s", err.Error())
	}
}

func TestNew_WithUnderlying(t *testing.T) {
	cause := stderrors.New("connection refused")
	err := New(ErrProvider, "api call failed", cause)

	if err.Error() != "api call failed: connection refused" {
		t.Errorf("unexpected Error(): %s", err.Error())
	}
	if !stderrors.Is(err, cause) {
		t.Error("expected Unwrap to return cause")
	}
}

func TestNewf(t *testing.T) {
	err := Newf(ErrNotFound, "user %d not found", 42)
	if err.Message != "user 42 not found" {
		t.Errorf("expected formatted message, got %s", err.Message)
	}
}

func TestWithField(t *testing.T) {
	err := New(ErrProvider, "fail", nil).
		WithField("provider", "openai").
		WithField("status", 500)

	if err.Fields["provider"] != "openai" {
		t.Errorf("expected provider=openai, got %v", err.Fields["provider"])
	}
	if err.Fields["status"] != 500 {
		t.Errorf("expected status=500, got %v", err.Fields["status"])
	}
}

func TestWithFields(t *testing.T) {
	err := New(ErrProvider, "fail", nil).
		WithFields(map[string]any{"a": 1, "b": 2})

	if len(err.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(err.Fields))
	}
}

func TestGetUserMessage(t *testing.T) {
	ue := New(ErrRateLimited, "429", nil)
	msg := GetUserMessage(ue)
	if msg != defaultMessages[ErrRateLimited] {
		t.Errorf("expected %q, got %q", defaultMessages[ErrRateLimited], msg)
	}

	// Non-UserError returns generic fallback
	plain := stderrors.New("boom")
	msg = GetUserMessage(plain)
	if msg != defaultMessages[ErrInternal] {
		t.Errorf("expected generic fallback, got %q", msg)
	}
}

func TestGetCode(t *testing.T) {
	ue := New(ErrTimeout, "slow", nil)
	if GetCode(ue) != ErrTimeout {
		t.Errorf("expected %s, got %s", ErrTimeout, GetCode(ue))
	}

	plain := stderrors.New("boom")
	if GetCode(plain) != ErrInternal {
		t.Errorf("expected ErrInternal for plain error, got %s", GetCode(plain))
	}
}

func TestIsCode(t *testing.T) {
	ue := New(ErrCancelled, "cancelled", nil)
	if !IsCode(ue, ErrCancelled) {
		t.Error("expected IsCode to match")
	}
	if IsCode(ue, ErrTimeout) {
		t.Error("expected IsCode not to match different code")
	}
	if IsCode(stderrors.New("x"), ErrCancelled) {
		t.Error("expected IsCode to return false for plain error")
	}
}

func TestRegisterMessages(t *testing.T) {
	custom := Code("custom_code")
	RegisterMessages(map[Code]string{
		custom: "Custom user message",
	})

	err := New(custom, "technical", nil)
	if err.UserMsg != "Custom user message" {
		t.Errorf("expected custom message, got %q", err.UserMsg)
	}

	// Override a default
	RegisterMessages(map[Code]string{
		ErrTimeout: "Overridden timeout message",
	})
	err2 := New(ErrTimeout, "slow", nil)
	if err2.UserMsg != "Overridden timeout message" {
		t.Errorf("expected overridden message, got %q", err2.UserMsg)
	}

	// Clean up so we don't affect other tests
	msgMu.Lock()
	delete(messages, custom)
	delete(messages, ErrTimeout)
	msgMu.Unlock()
}

func TestWrap(t *testing.T) {
	cause := stderrors.New("disk full")
	err := Wrap(ErrStorageWrite, "save settings", cause)

	if err.Code != ErrStorageWrite {
		t.Errorf("expected %s, got %s", ErrStorageWrite, err.Code)
	}
	if !stderrors.Is(err, cause) {
		t.Error("expected wrapped error to be unwrappable")
	}
}

func TestUnknownCode_FallsBackToInternal(t *testing.T) {
	unknown := Code("totally_unknown")
	err := New(unknown, "mystery", nil)
	if err.UserMsg != defaultMessages[ErrInternal] {
		t.Errorf("expected fallback to internal message, got %q", err.UserMsg)
	}
}
