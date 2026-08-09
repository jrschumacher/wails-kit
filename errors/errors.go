package errors

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"sync"

	"github.com/jrschumacher/wails-kit/v2/i18n"
)

// Code represents a unique error code for user-friendly messaging.
type Code string

// Common error codes. Apps extend with their own.
const (
	ErrAuthInvalid   Code = "auth_invalid"
	ErrAuthExpired   Code = "auth_expired"
	ErrAuthMissing   Code = "auth_missing"
	ErrNotFound      Code = "not_found"
	ErrPermission    Code = "permission_denied"
	ErrValidation    Code = "validation"
	ErrRateLimited   Code = "rate_limited"
	ErrTimeout       Code = "timeout"
	ErrCancelled     Code = "cancelled"
	ErrInternal      Code = "internal"
	ErrStorageRead   Code = "storage_read"
	ErrStorageWrite  Code = "storage_write"
	ErrConfigInvalid Code = "config_invalid"
	ErrConfigMissing Code = "config_missing"
	ErrProvider      Code = "provider_error"
)

// UserError represents an error with both technical and user-friendly messages.
type UserError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"` // Technical message for logs

	// UserMsg is the English/source-language user-facing message, captured
	// once at construction (New/Newf/Wrap always resolve it from Text.Other,
	// never from a localizer — see Text). Reading this field directly always
	// gets you the construction-time English snapshot; for the message
	// resolved against the currently installed localizer, use
	// GetUserMessage(err) or JSON-marshal the error (MarshalJSON resolves
	// this field live at marshal time). Kept for backward compatibility with
	// direct field access.
	UserMsg string `json:"userMsg"`

	// Text is the translatable user-facing message: a catalog key plus its
	// English fallback. GetUserMessage and MarshalJSON resolve it against
	// the package-level localizer (SetLocalizer) at read time.
	Text i18n.Text `json:"-"`

	Underlying error          `json:"-"`                // Original error (excluded from JSON)
	Fields     map[string]any `json:"fields,omitempty"` // Structured context
}

func (e *UserError) Error() string {
	if e.Underlying != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Underlying)
	}
	return e.Message
}

func (e *UserError) Unwrap() error {
	return e.Underlying
}

// MarshalJSON resolves UserMsg through the currently installed localizer
// (see GetUserMessage) rather than the construction-time English snapshot
// stored in the UserMsg field — this is what AD-5 means by "read-time
// resolution": errors are frequently constructed in init(), before any
// localizer exists, so the wire representation must re-resolve at marshal
// time, not carry whatever was true at construction.
func (e *UserError) MarshalJSON() ([]byte, error) {
	type alias UserError
	return json.Marshal(&struct {
		UserMsg string `json:"userMsg"`
		*alias
	}{
		UserMsg: GetUserMessage(e),
		alias:   (*alias)(e),
	})
}

// WithField adds a context field and returns the error for chaining.
func (e *UserError) WithField(key string, value any) *UserError {
	if e.Fields == nil {
		e.Fields = make(map[string]any)
	}
	e.Fields[key] = value
	return e
}

// WithFields adds multiple context fields.
func (e *UserError) WithFields(fields map[string]any) *UserError {
	if e.Fields == nil {
		e.Fields = make(map[string]any)
	}
	for k, v := range fields {
		e.Fields[k] = v
	}
	return e
}

// New creates a UserError with the given code and technical message.
// The user-facing message is looked up from the registered messages.
func New(code Code, message string, underlying error) *UserError {
	text := getUserText(code)
	return &UserError{
		Code:       code,
		Message:    message,
		UserMsg:    text.Other,
		Text:       text,
		Underlying: underlying,
		Fields:     make(map[string]any),
	}
}

// Newf creates a UserError with a formatted technical message.
// If the format string contains %w, the wrapped error is extracted as
// Underlying, exactly like fmt.Errorf/errors.Unwrap — an error argument
// paired with any other verb (%v, %s, ...) is not treated as wrapped.
func Newf(code Code, format string, args ...any) *UserError {
	fmtErr := fmt.Errorf(format, args...)
	text := getUserText(code)

	return &UserError{
		Code:       code,
		Message:    fmtErr.Error(),
		UserMsg:    text.Other,
		Text:       text,
		Underlying: stderrors.Unwrap(fmtErr),
		Fields:     make(map[string]any),
	}
}

// Wrap creates a UserError wrapping an existing error.
func Wrap(code Code, message string, err error) *UserError {
	return New(code, message, err)
}

// GetUserMessage extracts the user-friendly message from an error, resolved
// against the currently installed localizer (see SetLocalizer). For
// non-UserErrors, returns the generic ErrInternal fallback, also resolved.
func GetUserMessage(err error) string {
	var ue *UserError
	if stderrors.As(err, &ue) {
		return resolveText(ue.Text)
	}
	return resolveText(defaultMessages[ErrInternal])
}

// GetCode extracts the error code. Returns ErrInternal for non-UserErrors.
func GetCode(err error) Code {
	var ue *UserError
	if stderrors.As(err, &ue) {
		return ue.Code
	}
	return ErrInternal
}

// IsCode checks if an error matches a specific error code.
func IsCode(err error, code Code) bool {
	var ue *UserError
	if stderrors.As(err, &ue) {
		return ue.Code == code
	}
	return false
}

// --- Message registry ---

var (
	msgMu    sync.RWMutex
	messages = map[Code]i18n.Text{}
)

var defaultMessages = map[Code]i18n.Text{
	ErrAuthInvalid:   i18n.T("wailskit.errors.auth_invalid", "Authentication failed. Please check your credentials."),
	ErrAuthExpired:   i18n.T("wailskit.errors.auth_expired", "Your session has expired. Please reconnect."),
	ErrAuthMissing:   i18n.T("wailskit.errors.auth_missing", "Authentication is required. Please configure your credentials."),
	ErrNotFound:      i18n.T("wailskit.errors.not_found", "The requested item was not found."),
	ErrPermission:    i18n.T("wailskit.errors.permission_denied", "You don't have permission to perform this action."),
	ErrValidation:    i18n.T("wailskit.errors.validation", "The input is invalid. Please check and try again."),
	ErrRateLimited:   i18n.T("wailskit.errors.rate_limited", "Too many requests. Please wait and try again."),
	ErrTimeout:       i18n.T("wailskit.errors.timeout", "The operation timed out. Please try again."),
	ErrCancelled:     i18n.T("wailskit.errors.cancelled", "The operation was cancelled."),
	ErrInternal:      i18n.T("wailskit.errors.internal", "An unexpected error occurred. Please try again."),
	ErrStorageRead:   i18n.T("wailskit.errors.storage_read", "Failed to read data. Please try again."),
	ErrStorageWrite:  i18n.T("wailskit.errors.storage_write", "Failed to save data. Please try again."),
	ErrConfigInvalid: i18n.T("wailskit.errors.config_invalid", "Configuration is invalid. Please check your settings."),
	ErrConfigMissing: i18n.T("wailskit.errors.config_missing", "Required configuration is missing. Please check your settings."),
	ErrProvider:      i18n.T("wailskit.errors.provider_error", "The service provider returned an error. Please try again."),
}

// RegisterMessages adds or overrides user-facing messages for error codes.
// Apps and kit packages use this in init() to register domain-specific
// messages as i18n.Text — a stable catalog key plus its English fallback.
func RegisterMessages(msgs map[Code]i18n.Text) {
	msgMu.Lock()
	defer msgMu.Unlock()
	for k, v := range msgs {
		messages[k] = v
	}
}

// getUserText returns the registered (or default) i18n.Text for code,
// falling back to ErrInternal's for an unknown code. It never touches the
// localizer — that resolution happens later, at read time, in resolveText.
func getUserText(code Code) i18n.Text {
	msgMu.RLock()
	if t, ok := messages[code]; ok {
		msgMu.RUnlock()
		return t
	}
	msgMu.RUnlock()

	if t, ok := defaultMessages[code]; ok {
		return t
	}
	return defaultMessages[ErrInternal]
}

// --- Localizer wiring ---

var (
	locMu     sync.RWMutex
	localizer *i18n.Localizer
)

// SetLocalizer installs the package-level localizer used by GetUserMessage
// and UserError's JSON marshaling to resolve messages. Pass nil to clear it
// (GetUserMessage then falls back to each Text's English Other field — the
// same behavior as before any localizer was ever installed). Safe to call
// at any time, including after errors already exist: resolution happens at
// read time, not when a UserError was constructed, which matters because
// errors are frequently created in init(), before an app has wired up
// localization.
func SetLocalizer(l *i18n.Localizer) {
	locMu.Lock()
	defer locMu.Unlock()
	localizer = l
}

// resolveText resolves t against the currently installed localizer, or
// falls back to t.Other when none is installed — the "a consumer with no
// localizer still gets sensible English" guarantee.
func resolveText(t i18n.Text) string {
	locMu.RLock()
	l := localizer
	locMu.RUnlock()

	if l == nil {
		return t.Other
	}
	return l.T(t)
}
