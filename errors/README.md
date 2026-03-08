# errors

User-facing error types for Wails apps. Provides structured errors with both technical messages (for logs) and user-friendly messages (for UI), keyed by error codes.

## Usage

```go
import "github.com/jrschumacher/wails-kit/errors"

// Create errors with codes
err := errors.New(errors.ErrAuthExpired, "token expired at 2pm", nil)
err = errors.Newf(errors.ErrValidation, "field %s invalid", "email")
err = errors.Wrap(errors.ErrProvider, "anthropic failed", originalErr)

// Add structured context
err = err.WithField("provider", "anthropic").WithField("status", 429)

// Extract user-facing info from any error
msg := errors.GetUserMessage(err)  // "Your session has expired. Please reconnect."
code := errors.GetCode(err)        // errors.ErrAuthExpired
errors.IsCode(err, errors.ErrRateLimited)  // false
```

## Built-in error codes

| Code | Default user message |
|------|---------------------|
| `auth_invalid` | Authentication failed. Please check your credentials. |
| `auth_expired` | Your session has expired. Please reconnect. |
| `auth_missing` | Authentication is required. Please configure your credentials. |
| `not_found` | The requested item was not found. |
| `permission_denied` | You don't have permission to perform this action. |
| `validation` | The input is invalid. Please check and try again. |
| `rate_limited` | Too many requests. Please wait and try again. |
| `timeout` | The operation timed out. Please try again. |
| `cancelled` | The operation was cancelled. |
| `internal` | An unexpected error occurred. Please try again. |
| `storage_read` | Failed to read data. Please try again. |
| `storage_write` | Failed to save data. Please try again. |
| `config_invalid` | Configuration is invalid. Please check your settings. |
| `config_missing` | Required configuration is missing. Please check your settings. |
| `provider_error` | The service provider returned an error. Please try again. |

## Custom error codes

Apps register their own codes and messages:

```go
errors.RegisterMessages(map[errors.Code]string{
    "jira_unreachable": "Cannot reach Jira. Check your network connection.",
    "sync_conflict":    "A sync conflict occurred. Please resolve it manually.",
})

err := errors.New("jira_unreachable", "connection refused to jira.example.com", netErr)
```

## UserError type

```go
type UserError struct {
    Code       Code           // Error code for programmatic handling
    Message    string         // Technical message for logs
    UserMsg    string         // User-friendly message for UI
    Underlying error          // Original error (for Unwrap)
    Fields     map[string]any // Structured context
}
```

`UserError` implements the standard `error` interface and supports `errors.Unwrap` for error chain inspection.

## Extracting error info

These functions work with any `error`, returning sensible defaults for non-`UserError` values:

- `GetUserMessage(err)` — returns the user-friendly message, or the `internal` default
- `GetCode(err)` — returns the error code, or `ErrInternal`
- `IsCode(err, code)` — checks if the error matches a specific code
