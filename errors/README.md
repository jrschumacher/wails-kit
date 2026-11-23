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
msg := errors.GetUserMessage(err)  // "Sign-in expired"
code := errors.GetCode(err)        // errors.ErrAuthExpired
errors.IsCode(err, errors.ErrRateLimited)  // false
```

## Built-in error codes

The defaults follow the voice in `frontend/ui/BEHAVIOUR.md` ("Writing the words"):
short fragments with no full stop that name what happened, in the person's words.
They never say "Please try again" or "Something went wrong": what happens next (Retry,
"Next try in 8 s") is the UI's to say. Apps override them, or add their own, with
`RegisterMessages`.

| Code | Default user message |
|------|---------------------|
| `auth_invalid` | Couldn't sign in with these credentials |
| `auth_expired` | Sign-in expired |
| `auth_missing` | Sign-in needed |
| `not_found` | Couldn't find it |
| `permission_denied` | You don't have permission for this |
| `validation` | Some values aren't valid |
| `rate_limited` | Too many requests right now |
| `timeout` | Timed out |
| `cancelled` | Cancelled |
| `internal` | Couldn't finish this |
| `storage_read` | Couldn't read the data |
| `storage_write` | Couldn't save the data |
| `config_invalid` | A setting isn't valid |
| `config_missing` | A required setting is missing |
| `provider_error` | The service returned an error |

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
