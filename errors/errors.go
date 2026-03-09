package errors

import (
	stderrors "errors"
	"fmt"
	"sync"
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
	Code       Code           `json:"code"`
	Message    string         `json:"message"`              // Technical message for logs
	UserMsg    string         `json:"userMsg"`              // User-friendly message for UI
	Underlying error          `json:"-"`                    // Original error (excluded from JSON)
	Fields     map[string]any `json:"fields,omitempty"`     // Structured context
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
	return &UserError{
		Code:       code,
		Message:    message,
		UserMsg:    getUserMsg(code),
		Underlying: underlying,
		Fields:     make(map[string]any),
	}
}

// Newf creates a UserError with a formatted technical message.
// If any argument is an error and the format string contains %w,
// the error is extracted as the Underlying error (like fmt.Errorf).
func Newf(code Code, format string, args ...any) *UserError {
	// Extract a wrapped error if %w is used, similar to fmt.Errorf.
	var underlying error
	for i, arg := range args {
		if err, ok := arg.(error); ok {
			// Check if this corresponds to a %w verb by using fmt.Errorf
			// and seeing if we can unwrap it.
			_ = i
			underlying = err
			break
		}
	}

	// Use fmt.Errorf to handle %w formatting correctly, then extract
	// the message and underlying error.
	fmtErr := fmt.Errorf(format, args...)
	msg := fmtErr.Error()
	unwrapped := stderrors.Unwrap(fmtErr)
	if unwrapped != nil {
		underlying = unwrapped
	}

	return &UserError{
		Code:       code,
		Message:    msg,
		UserMsg:    getUserMsg(code),
		Underlying: underlying,
		Fields:     make(map[string]any),
	}
}

// Wrap creates a UserError wrapping an existing error.
func Wrap(code Code, message string, err error) *UserError {
	return New(code, message, err)
}

// GetUserMessage extracts the user-friendly message from an error.
// For non-UserErrors, returns a generic fallback.
func GetUserMessage(err error) string {
	var ue *UserError
	if stderrors.As(err, &ue) {
		return ue.UserMsg
	}
	return defaultMessages[ErrInternal]
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
	messages = map[Code]string{}
)

var defaultMessages = map[Code]string{
	ErrAuthInvalid:   "Authentication failed. Please check your credentials.",
	ErrAuthExpired:   "Your session has expired. Please reconnect.",
	ErrAuthMissing:   "Authentication is required. Please configure your credentials.",
	ErrNotFound:      "The requested item was not found.",
	ErrPermission:    "You don't have permission to perform this action.",
	ErrValidation:    "The input is invalid. Please check and try again.",
	ErrRateLimited:   "Too many requests. Please wait and try again.",
	ErrTimeout:       "The operation timed out. Please try again.",
	ErrCancelled:     "The operation was cancelled.",
	ErrInternal:      "An unexpected error occurred. Please try again.",
	ErrStorageRead:   "Failed to read data. Please try again.",
	ErrStorageWrite:  "Failed to save data. Please try again.",
	ErrConfigInvalid: "Configuration is invalid. Please check your settings.",
	ErrConfigMissing: "Required configuration is missing. Please check your settings.",
	ErrProvider:      "The service provider returned an error. Please try again.",
}

// RegisterMessages adds or overrides user-facing messages for error codes.
// Apps use this to register domain-specific messages.
func RegisterMessages(msgs map[Code]string) {
	msgMu.Lock()
	defer msgMu.Unlock()
	for k, v := range msgs {
		messages[k] = v
	}
}

func getUserMsg(code Code) string {
	msgMu.RLock()
	if msg, ok := messages[code]; ok {
		msgMu.RUnlock()
		return msg
	}
	msgMu.RUnlock()

	if msg, ok := defaultMessages[code]; ok {
		return msg
	}
	return defaultMessages[ErrInternal]
}
