# errors

User-facing error types for Wails apps. Provides structured errors with both technical messages (for logs) and user-friendly messages (for UI), keyed by error codes.

## Usage

```go
import "github.com/jrschumacher/wails-kit/v2/errors"

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

`errors.SetLocalizer(l *i18n.Localizer)` is an optional package-level call — see
[Localization (i18n)](#localization-i18n) below. Skip it and every message resolves to
its literal English fallback; nothing else about this snippet changes.

## Built-in error codes

Codes are a **wire contract** — Prune's frontend branches on the literal string value
(`app/frontend/src/lib/errors.ts`). Localizing the *text* below never changes a code's
string value; see [Localization (i18n)](#localization-i18n) for how the text becomes
translatable.

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

Apps register their own codes and messages as `i18n.Text` — a stable catalog key plus
its English fallback (package [`i18n`](../i18n/README.md)):

```go
errors.RegisterMessages(map[errors.Code]i18n.Text{
    "jira_unreachable": i18n.T("myapp.errors.jira_unreachable", "Cannot reach Jira. Check your network connection."),
    "sync_conflict":    i18n.T("myapp.errors.sync_conflict", "A sync conflict occurred. Please resolve it manually."),
})

err := errors.New("jira_unreachable", "connection refused to jira.example.com", netErr)
```

## UserError type

```go
type UserError struct {
    Code       Code           // Error code for programmatic handling
    Message    string         // Technical message for logs
    UserMsg    string         // English snapshot captured at construction — see below
    Text       i18n.Text      // Translatable message: catalog key + English fallback
    Underlying error          // Original error (for Unwrap)
    Fields     map[string]any // Structured context
}
```

`UserError` implements the standard `error` interface and supports `errors.Unwrap` for
error chain inspection.

**`UserMsg` is a construction-time English snapshot, not a live value.** `New`/`Newf`/
`Wrap` always set it from `Text.Other`, never from a localizer — errors are frequently
constructed in a package `init()`, before any app has had a chance to call
`SetLocalizer`. For the message resolved against whatever localizer is installed *right
now*, use `GetUserMessage(err)` (or JSON-marshal the error — see below), not the
`UserMsg` field directly.

## Extracting error info

These functions work with any `error`, returning sensible defaults for non-`UserError` values:

- `GetUserMessage(err)` — returns the user-friendly message, resolved through the
  package-level localizer (`SetLocalizer`) if one is installed, else `Text.Other`
- `GetCode(err)` — returns the error code, or `ErrInternal`
- `IsCode(err, code)` — checks if the error matches a specific code

## Localization (i18n)

`errors.SetLocalizer(l *i18n.Localizer)` installs a package-level localizer used by
`GetUserMessage` and `UserError.MarshalJSON` to resolve messages. There is no per-error
localizer option — errors are frequently constructed before an app has finished wiring
its dependencies, so this package resolves lazily, at *read* time, against whichever
localizer is installed when you ask, not whatever was installed (or absent) when the
error was made:

```go
l, _ := i18n.New(i18n.WithCatalog(myAppLocales))
errors.SetLocalizer(l)

err := errors.New(errors.ErrNotFound, "row missing", nil) // fine even before SetLocalizer
errors.GetUserMessage(err) // resolved through l's current locale
```

**Localization is opt-in.** Never call `SetLocalizer` (or call it with `nil`) and every
message resolves to its `i18n.Text.Other` value — the same English text this package
has always returned. This is the "a consumer with no localizer must still get sensible
English" guarantee: `resolveText`'s entire contract is `nil` localizer → `t.Other`;
non-nil → `l.T(t)`.

**JSON marshaling resolves live, too.** `UserError` implements `MarshalJSON`: the
`userMsg` field in the wire representation is always `GetUserMessage(err)` at marshal
time, not the `UserMsg` field's construction-time snapshot. A frontend that only ever
sees errors via JSON (e.g. over a Wails binding) gets live localization without any
special handling.

**Switch locale at runtime** with `l.SetLocale("fr")` — the next `GetUserMessage` call
(or JSON marshal) reflects it immediately; nothing needs to be reconstructed.

**Dependency direction.** `errors` imports `i18n`, one-way. `i18n` never imports
`errors` — see [`AGENTS.md`](./AGENTS.md) for why that direction matters.

## Package catalogs

Every kit package that calls `RegisterMessages` ships its own `locales/en.json` next to
its source, keyed `wailskit.<package>.<code>`, mirroring this package's own
`errors/locales/en.json`. Apps that call `RegisterMessages` should do the same with
their own namespace (not `wailskit.`, which is reserved).
