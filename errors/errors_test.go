package errors

import (
	"encoding/json"
	stderrors "errors"
	"testing"
	"testing/fstest"

	"github.com/jrschumacher/wails-kit/v2/i18n"
)

func TestNew(t *testing.T) {
	err := New(ErrAuthInvalid, "bad token", nil)
	if err.Code != ErrAuthInvalid {
		t.Errorf("expected code %s, got %s", ErrAuthInvalid, err.Code)
	}
	if err.Message != "bad token" {
		t.Errorf("expected message 'bad token', got %s", err.Message)
	}
	if err.UserMsg != defaultMessages[ErrAuthInvalid].Other {
		t.Errorf("expected user message %q, got %q", defaultMessages[ErrAuthInvalid].Other, err.UserMsg)
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

func TestNewf_WithWrappedError(t *testing.T) {
	cause := stderrors.New("disk full")
	err := Newf(ErrStorageWrite, "save failed: %w", cause)

	if err.Underlying == nil {
		t.Fatal("expected Underlying to be set when using %w")
	}
	if !stderrors.Is(err, cause) {
		t.Error("expected wrapped error to be unwrappable via Is")
	}
	if err.Message != "save failed: disk full" {
		t.Errorf("unexpected message: %s", err.Message)
	}
}

// TestNewf_NoWrapWithoutWVerb is the failing-first regression test for the
// known defect: Newf used to grab the first error argument as Underlying
// even when the format verb was %v or %s, contradicting its own doc comment
// (which says that only happens for %w, like fmt.Errorf). On the buggy
// implementation this failed because Underlying was non-nil.
func TestNewf_NoWrapWithoutWVerb(t *testing.T) {
	cause := stderrors.New("disk full")

	errV := Newf(ErrStorageWrite, "save failed: %v", cause)
	if errV.Underlying != nil {
		t.Errorf("%%v: expected no Underlying, got %v", errV.Underlying)
	}
	if errV.Message != "save failed: disk full" {
		t.Errorf("%%v: unexpected message: %s", errV.Message)
	}

	errS := Newf(ErrStorageWrite, "save failed: %s", cause)
	if errS.Underlying != nil {
		t.Errorf("%%s: expected no Underlying, got %v", errS.Underlying)
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
	if msg != defaultMessages[ErrRateLimited].Other {
		t.Errorf("expected %q, got %q", defaultMessages[ErrRateLimited].Other, msg)
	}

	// Non-UserError returns generic fallback
	plain := stderrors.New("boom")
	msg = GetUserMessage(plain)
	if msg != defaultMessages[ErrInternal].Other {
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
	RegisterMessages(map[Code]i18n.Text{
		custom: i18n.T("test.errors.custom_code", "Custom user message"),
	})

	err := New(custom, "technical", nil)
	if err.UserMsg != "Custom user message" {
		t.Errorf("expected custom message, got %q", err.UserMsg)
	}

	// Override a default
	RegisterMessages(map[Code]i18n.Text{
		ErrTimeout: i18n.T("test.errors.timeout_override", "Overridden timeout message"),
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
	if err.UserMsg != defaultMessages[ErrInternal].Other {
		t.Errorf("expected fallback to internal message, got %q", err.UserMsg)
	}
}

// --- i18n integration ---

// TestNilLocalizerFallsBack is the "no localizer installed" contract: every
// consumer that never calls SetLocalizer must still get sensible English
// (Text.Other), never an empty string or a bare key.
func TestNilLocalizerFallsBack(t *testing.T) {
	SetLocalizer(nil) // ensure a clean slate regardless of test order
	t.Cleanup(func() { SetLocalizer(nil) })

	err := New(ErrNotFound, "lookup failed", nil)
	if got, want := GetUserMessage(err), defaultMessages[ErrNotFound].Other; got != want {
		t.Errorf("GetUserMessage() = %q, want %q", got, want)
	}
}

// TestResolveAtReadTime proves resolution happens when GetUserMessage (or
// JSON marshaling) is called, not when the UserError was constructed — the
// property AD-5 requires because kit/app errors are frequently registered
// and constructed in init(), before any localizer exists yet.
func TestResolveAtReadTime(t *testing.T) {
	t.Cleanup(func() { SetLocalizer(nil) })

	// Constructed with no localizer installed at all.
	SetLocalizer(nil)
	err := New(ErrNotFound, "lookup failed", nil)

	// A localizer with a Spanish translation is installed only *after*
	// construction.
	fsys := fstest.MapFS{
		"locales/es.json": &fstest.MapFile{
			Data: []byte(`{"wailskit.errors.not_found": "El elemento solicitado no fue encontrado."}`),
		},
	}
	loc, err2 := i18n.New(i18n.WithCatalog(fsys), i18n.WithLocale("es"))
	if err2 != nil {
		t.Fatalf("i18n.New: %v", err2)
	}
	SetLocalizer(loc)

	const want = "El elemento solicitado no fue encontrado."
	if got := GetUserMessage(err); got != want {
		t.Errorf("GetUserMessage() after SetLocalizer = %q, want %q", got, want)
	}

	// The construction-time UserMsg snapshot is unaffected (English, as
	// documented) — only read-time resolution changes.
	if err.UserMsg != defaultMessages[ErrNotFound].Other {
		t.Errorf("UserMsg field changed after SetLocalizer: %q", err.UserMsg)
	}
}

// TestGetUserMessage_ResolvesThroughCatalog covers a localized message
// resolving through a real catalog (not just falling back to Text.Other).
func TestGetUserMessage_ResolvesThroughCatalog(t *testing.T) {
	t.Cleanup(func() { SetLocalizer(nil) })

	fsys := fstest.MapFS{
		"locales/fr.json": &fstest.MapFile{
			Data: []byte(`{"wailskit.errors.timeout": "L'opération a expiré. Veuillez réessayer."}`),
		},
	}
	loc, err := i18n.New(i18n.WithCatalog(fsys), i18n.WithLocale("fr"))
	if err != nil {
		t.Fatalf("i18n.New: %v", err)
	}
	SetLocalizer(loc)

	ue := New(ErrTimeout, "slow", nil)
	const want = "L'opération a expiré. Veuillez réessayer."
	if got := GetUserMessage(ue); got != want {
		t.Errorf("GetUserMessage() = %q, want %q", got, want)
	}
}

// TestLocaleSwitching covers SetLocale changing which catalog entry
// GetUserMessage resolves through, without constructing a new error.
func TestLocaleSwitching(t *testing.T) {
	t.Cleanup(func() { SetLocalizer(nil) })

	fsys := fstest.MapFS{
		"locales/es.json": &fstest.MapFile{
			Data: []byte(`{"wailskit.errors.validation": "La entrada no es válida."}`),
		},
		"locales/fr.json": &fstest.MapFile{
			Data: []byte(`{"wailskit.errors.validation": "L'entrée n'est pas valide."}`),
		},
	}
	loc, err := i18n.New(i18n.WithCatalog(fsys), i18n.WithLocale("es"))
	if err != nil {
		t.Fatalf("i18n.New: %v", err)
	}
	SetLocalizer(loc)

	ue := New(ErrValidation, "bad input", nil)
	if got, want := GetUserMessage(ue), "La entrada no es válida."; got != want {
		t.Errorf("before switch: GetUserMessage() = %q, want %q", got, want)
	}

	if err := loc.SetLocale("fr"); err != nil {
		t.Fatalf("SetLocale: %v", err)
	}

	if got, want := GetUserMessage(ue), "L'entrée n'est pas valide."; got != want {
		t.Errorf("after switch: GetUserMessage() = %q, want %q", got, want)
	}
}

// TestMarshalJSON_ResolvesLive covers the wire contract: JSON-marshaling a
// UserError resolves userMsg through the currently installed localizer, not
// the construction-time English snapshot.
func TestMarshalJSON_ResolvesLive(t *testing.T) {
	t.Cleanup(func() { SetLocalizer(nil) })

	fsys := fstest.MapFS{
		"locales/de.json": &fstest.MapFile{
			Data: []byte(`{"wailskit.errors.permission_denied": "Sie haben keine Berechtigung."}`),
		},
	}
	loc, err := i18n.New(i18n.WithCatalog(fsys), i18n.WithLocale("de"))
	if err != nil {
		t.Fatalf("i18n.New: %v", err)
	}
	SetLocalizer(loc)

	ue := New(ErrPermission, "not allowed", nil)
	data, err := json.Marshal(ue)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		UserMsg string `json:"userMsg"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.UserMsg != "Sie haben keine Berechtigung." {
		t.Errorf("userMsg = %q, want German translation", decoded.UserMsg)
	}
	if decoded.Code != string(ErrPermission) {
		t.Errorf("code = %q, want %q", decoded.Code, ErrPermission)
	}
}

// TestCodesUnchanged pins the wire contract: Code values are a stable
// contract consumed by frontend branching logic (Prune's
// app/frontend/src/lib/errors.ts). Localizing message *text* must never
// change a code's string value.
func TestCodesUnchanged(t *testing.T) {
	want := map[Code]string{
		ErrAuthInvalid:   "auth_invalid",
		ErrAuthExpired:   "auth_expired",
		ErrAuthMissing:   "auth_missing",
		ErrNotFound:      "not_found",
		ErrPermission:    "permission_denied",
		ErrValidation:    "validation",
		ErrRateLimited:   "rate_limited",
		ErrTimeout:       "timeout",
		ErrCancelled:     "cancelled",
		ErrInternal:      "internal",
		ErrStorageRead:   "storage_read",
		ErrStorageWrite:  "storage_write",
		ErrConfigInvalid: "config_invalid",
		ErrConfigMissing: "config_missing",
		ErrProvider:      "provider_error",
	}
	for code, wantStr := range want {
		if string(code) != wantStr {
			t.Errorf("Code %v = %q, want %q", code, string(code), wantStr)
		}
	}
}
