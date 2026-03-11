# wails-kit

Reusable Go module for Wails v3 apps. Provides a schema-driven settings framework, OS keyring integration, SQLite database management with migrations, structured logging, typed events, user-facing error types, GitHub-based auto-updates, native menu shortcuts, and diagnostics bundle creation.

## Philosophy

wails-kit provides **desktop app infrastructure** — the plumbing that every Wails app needs but shouldn't rewrite. Packages belong here when they:

- Solve a problem **specific to desktop apps or Wails integration** (OS paths, keyring, window lifecycle)
- Eliminate **real boilerplate** that multiple apps would otherwise copy-paste
- Provide **infrastructure, not business logic** (storage, config, lifecycle — not domain models or UI)

Generic Go libraries (AI SDKs, HTTP clients, data processing) belong in standalone repos, not in the kit.

**LLM support:** For LLM provider integration, we recommend [any-llm-go](https://github.com/mozilla-ai/any-llm-go). wails-kit provides a settings template (`settings/templates/anyllm`) for wiring any-llm-go configuration into the schema-driven settings framework.

## Packages

### [`appdirs`](appdirs/README.md) — OS-Standard App Directories

Unified OS-standard directory paths for config, data, cache, log, and temp. Replaces duplicated path logic across packages.

```go
import "github.com/jrschumacher/wails-kit/appdirs"

dirs := appdirs.New("my-app")

dirs.Config()  // ~/Library/Application Support/my-app/ (macOS)
dirs.Data()    // persistent user data
dirs.Cache()   // ~/Library/Caches/my-app/ (macOS)
dirs.Log()     // ~/Library/Logs/my-app/ (macOS)
dirs.Temp()    // temporary working files

dirs.EnsureAll()  // create all dirs with 0700 permissions
dirs.CleanTemp()  // remove stale temp files on startup
```

### [`keyring`](keyring/README.md) — OS Keyring Credential Storage

Generic OS keyring wrapper with environment variable fallback. Used internally by settings for password fields, and available directly for app-specific secrets.

```go
import "github.com/jrschumacher/wails-kit/keyring"

// OS keyring with env var fallback (e.g. MYAPP_API_KEY)
store := keyring.NewOSStore("my-app", keyring.WithEnvPrefix("MYAPP"))

store.Set("api_key", "sk-abc123")
val, err := store.Get("api_key")
store.Has("api_key")  // true
store.Delete("api_key")

// Store structured data (OAuth tokens, etc.)
keyring.SetJSON(store, "oauth_token", myToken)
keyring.GetJSON(store, "oauth_token", &token)

// In-memory store for tests
testStore := keyring.NewMemoryStore()
```

**Env var fallback:** `Get("api_key")` checks the keyring first. If not found and an env prefix is configured, it checks `MYAPP_API_KEY` (uppercased, dots/dashes become underscores). This enables headless/CI operation without an OS keyring.

### [`settings`](settings/README.md) — Schema-Driven Settings

The backend defines a settings schema (fields, types, options, visibility conditions) and any frontend renders from it dynamically.

**Storage paths** (OS-standard via `os.UserConfigDir()`):
- macOS: `~/Library/Application Support/{app}/settings.json`
- Linux: `$XDG_CONFIG_HOME/{app}/settings.json`
- Windows: `%AppData%/{app}/settings.json`

```go
import (
    "github.com/jrschumacher/wails-kit/settings"
    "github.com/jrschumacher/wails-kit/keyring"
)

store := keyring.NewOSStore("my-app", keyring.WithEnvPrefix("MYAPP"))

svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithKeyring(store),                 // OS keyring for password fields
    settings.WithGroup(mySettingsGroup()),
    settings.WithOnChange(func(v map[string]any) {
        // react to settings changes
    }),
)

// Register as a Wails service
app := application.New(application.Options{
    Services: []application.Service{
        application.NewService(svc),
    },
})
```

**Frontend contract** (3 Wails bindings):
- `GetSchema()` — returns JSON describing all fields, types, options, conditions
- `GetValues()` — returns current values with defaults and computed fields
- `SetValues(values)` — validates, saves, triggers onChange callbacks

**Defining a settings group:**

```go
func mySettingsGroup() settings.Group {
    return settings.Group{
        Key:   "appearance",
        Label: "Appearance",
        Fields: []settings.Field{
            {
                Key:     "appearance.theme",
                Type:    settings.FieldSelect,
                Label:   "Theme",
                Default: "system",
                Options: []settings.SelectOption{
                    {Label: "System", Value: "system"},
                    {Label: "Light", Value: "light"},
                    {Label: "Dark", Value: "dark"},
                },
            },
        },
    }
}
```

**Password fields** are stored in the OS keyring, never in the JSON file:

```go
settings.Field{
    Key:   "api.token",
    Type:  settings.FieldPassword,
    Label: "API Token",
}
```

- `GetValues()` returns `"••••••••"` for set passwords, `""` for unset
- `SetValues()` with `"••••••••"` is a no-op (user didn't change it)
- `SetValues()` with `""` clears the secret from keyring
- `GetSecret(key)` returns the actual value (backend use only)

**Schema features:**
- `Field.DynamicOptions` — select options that change based on another field's value
- `Field.Condition` — show/hide field based on another field's value
- `Field.Advanced` — render behind a "Show advanced" toggle
- `Field.Validation` — required, pattern, min/max length, min/max number
- `Group.ComputeFuncs` — server-side computed readonly fields

**CLI/headless mode** — see [`settings/cli`](settings/cli/README.md) for non-interactive get/set/show without a GUI (CI/scripting).

**Other behaviors:**
- **Atomic writes** — writes to `.tmp` then renames, preventing corruption on crash
- **Schema migration** — unknown keys in saved files are stripped on load
- **File permissions** — directories 0700, settings file 0600

### [`llm`](llm/README.md) — LLM Provider Management

Provider interface, factory pattern, and a built-in settings group for LLM configuration.

```go
import (
    "github.com/jrschumacher/wails-kit/llm"
    "github.com/jrschumacher/wails-kit/settings"
    _ "github.com/jrschumacher/wails-kit/llm/anthropic"
    _ "github.com/jrschumacher/wails-kit/llm/openai"
)

svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithGroup(llm.LLMSettingsGroup()),  // adds provider/model/advanced fields
)

mgr := llm.NewProviderManager(svc)

// Get the provider (lazy-initialized from settings)
provider, err := mgr.Provider()

// Stream a chat
provider.StreamChat(ctx, llm.ChatRequest{
    SystemPrompt: "You are helpful.",
    Messages:     []llm.ChatMessage{{Role: "user", Content: "Hello"}},
}, func(event llm.StreamEvent) {
    switch event.Type {
    case "delta":
        fmt.Print(event.Text)
    case "done":
        fmt.Println()
    }
})

// After settings change, reload the provider
mgr.Reload()
```

**Built-in providers:**
- `llm/anthropic` — Anthropic SDK with Cloudflare AI Gateway support
- `llm/openai` — OpenAI SDK with Cloudflare AI Gateway support
- `llm/mock` — Mock provider for testing

Providers self-register via `init()`. Import with blank identifier to activate.

**LLM settings group fields:**
- Provider selection (Anthropic / OpenAI)
- Model selection (dynamic by provider)
- Per-provider advanced: base URL, API key, API format, custom model ID
- Computed resolved model ID

#### Context Window Builder

Manages conversation history for LLM chat with bounded context windows.

```go
cb := llm.NewContextBuilder("You are a helpful assistant.")
cb.WindowSize = 20  // keep last 20 messages (default)
cb.MaxTokens = 4096 // default

// Optionally add widget/page context to the system prompt
cb.SetWidgetContext("User is viewing issue ABC-123")

// Build a request from full conversation history
req := cb.BuildRequest(allMessages)
// req.SystemPrompt includes base prompt + widget context
// req.Messages is windowed with older messages summarized
// req.MaxTokens is set

provider.StreamChat(ctx, req, handler)
```

**Behavior:**
- Sliding window keeps the last N messages
- Tool-use / tool-result pairs are kept atomic (never split across the window boundary)
- Older messages beyond the window are summarized into a synthetic message
- Summary caps at 8 topics with content truncated to 100 chars

### [`llm/mock`](llm/README.md#mock-provider) — Test Helper

```go
import "github.com/jrschumacher/wails-kit/llm/mock"

p := &mock.Provider{
    Name:  "test",
    Model: "test-model",
    OnStreamChat: func(ctx context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error {
        handler(llm.StreamEvent{Type: "delta", Text: "test response"})
        handler(llm.StreamEvent{Type: "done", StopReason: "end_turn"})
        return nil
    },
}
```

### [`database`](database/README.md) — SQLite Database with Migrations

SQLite database management with [goose](https://github.com/pressly/goose) migrations. Pure-Go SQLite driver (no CGO), OS-standard database paths, WAL mode and sensible pragmas out of the box.

```go
import (
    "embed"
    "github.com/jrschumacher/wails-kit/database"
)

//go:embed migrations/*.sql
var migrations embed.FS

db, err := database.New(
    database.WithAppName("my-app"),        // uses appdirs for OS path
    database.WithMigrations(migrations),   // goose SQL migrations
)
defer db.Close()

// Access the underlying *sql.DB
db.DB().QueryRow("SELECT ...")

// Check schema version
version, _ := db.Version()
```

**Features:**
- Goose-managed SQL migrations with `embed.FS` support
- OS-standard data directory via `appdirs`
- WAL mode, foreign keys, busy timeout enabled by default
- Bring your own `*sql.DB` with `WithDB()`

**Events:** `database:migrated`

**Error codes:** `database_open`, `database_migrate`

See [`database/README.md`](database/README.md) for full documentation.

### [`diagnostics`](diagnostics/README.md) — Support Bundle Creation

Collects application state, logs, and system info into a shareable zip bundle for crash reporting and user support. Integrates with `appdirs`, `settings`, and `logging`.

```go
import "github.com/jrschumacher/wails-kit/diagnostics"

svc, err := diagnostics.NewService(
    diagnostics.WithAppName("my-app"),
    diagnostics.WithVersion("1.2.3"),
    diagnostics.WithDirs(dirs),            // log directory from appdirs
    diagnostics.WithSettings(settingsSvc), // sanitized settings snapshot
)

// Create a zip bundle for the user to share
path, err := svc.CreateBundle(ctx, outputDir)

// Get system info for an About screen
info := svc.GetSystemInfo()
```

**Bundle contents:** `system.json` (OS, arch, versions), `settings.json` (passwords redacted), `logs/` (recent log files with size cap), `manifest.txt` (file listing for user review).

**Events:** `diagnostics:bundle_created`

**Error codes:** `diagnostics_bundle`, `diagnostics_logs`

### [`shortcuts`](shortcuts/README.md) — Native Menu Shortcuts

Standard keyboard shortcuts and native menu bar for Wails v3 apps. Handles platform differences automatically and emits events via the kit event system.

```go
import "github.com/jrschumacher/wails-kit/shortcuts"

mgr := shortcuts.New(
    shortcuts.WithDefaults(),   // App, File, Edit, View, Window menus
    shortcuts.WithSettings(),   // ⌘, / Ctrl+, → emits "settings:open"
    shortcuts.WithEmitter(emitter),
)
mgr.Apply(app)
```

**Features:**
- Standard menus: App (macOS), File, Edit, View, Window
- Settings shortcut: ⌘, (macOS) / Ctrl+, (others) → emits `settings:open`
- Platform-correct placement (App menu on macOS, Edit menu elsewhere)

**Events:** `settings:open`

See [`shortcuts/README.md`](shortcuts/README.md) for full documentation.

### [`lifecycle`](lifecycle/README.md) — Service Lifecycle Manager

Ordered startup and shutdown of services with dependency tracking and partial failure rollback.

```go
import "github.com/jrschumacher/wails-kit/lifecycle"

mgr, err := lifecycle.NewManager(
    lifecycle.WithService("database", dbService),
    lifecycle.WithService("settings", settingsService, lifecycle.DependsOn("database")),
    lifecycle.WithService("storage", storageService, lifecycle.DependsOn("database")),
    lifecycle.WithService("updates", updateService, lifecycle.DependsOn("settings")),
)

err = mgr.Startup(ctx)   // starts in dependency order; rolls back on failure
err = mgr.Shutdown()     // stops in reverse order; collects all errors
```

**Features:**
- Topological sort with cycle detection at construction time
- Partial failure rollback — if service N fails, services 1..N-1 are shut down
- All-errors shutdown — continues through failures, joins all errors
- Events: `lifecycle:started`, `lifecycle:stopped`, `lifecycle:error`, `lifecycle:rollback`
- Error codes: `lifecycle_cyclic_dependency`, `lifecycle_missing_dependency`, `lifecycle_startup`, `lifecycle_shutdown`

### [`state`](state/README.md) — Generic Typed State Persistence

Lightweight typed state persistence to disk. Save/load any struct as JSON with atomic writes, without schema, validation, or keyring integration. See [`state/README.md`](state/README.md).

### [`errors`](errors/README.md) — User-Facing Error Types

Generic error types for Wails apps. Apps add their own domain-specific codes and messages.

```go
import "github.com/jrschumacher/wails-kit/errors"

// Create errors with codes
err := errors.New(errors.ErrAuthExpired, "token expired at 2pm", nil)
err = errors.Newf(errors.ErrValidation, "field %s invalid", "email")
err = errors.Wrap(errors.ErrProviderError, "anthropic failed", originalErr)

// Add structured context
err = err.WithField("provider", "anthropic").WithField("status", 429)

// Extract user-facing info from any error
msg := errors.GetUserMessage(err)  // "Your session has expired. Please sign in again."
code := errors.GetCode(err)        // errors.ErrAuthExpired
errors.IsCode(err, errors.ErrRateLimited)  // false

// Register app-specific messages
errors.RegisterMessages(map[errors.Code]string{
    "jira_unreachable": "Cannot reach Jira. Check your network connection.",
})
```

**Built-in codes:** `auth_invalid`, `auth_expired`, `auth_missing`, `not_found`, `permission_denied`, `validation`, `rate_limited`, `timeout`, `cancelled`, `internal`, `storage_read`, `storage_write`, `config_invalid`, `config_missing`, `provider_error`

Each code has a default user-facing message. Apps override or extend via `RegisterMessages()`.

### [`logging`](logging/README.md) — Structured Logging

OS-aware structured logging with file rotation and sensitive field redaction. Built on `slog` with JSON output.

```go
import "github.com/jrschumacher/wails-kit/logging"

err := logging.Init(&logging.Config{
    AppName:       "my-app",
    Level:         "info",         // debug, info, warn, error
    AddSource:     true,
    MaxSize:       100,            // MB per file
    MaxAge:        7,              // days
    MaxBackups:    10,
    Compress:      true,
    SensitiveKeys: []string{"password", "token", "api_key"},
})

// Package-level convenience
logging.Info("server started", "port", 8080)
logging.Error("request failed", err, "path", "/api/data")
logging.Debug("cache hit", "key", "user:123")

// Logger with preset fields
logger := logging.Get().WithFields("component", "sync", "user_id", "abc")
logger.Info("sync started")
```

**Log paths** (OS-specific):
- macOS: `~/Library/Logs/{app}/`
- Linux: `$XDG_STATE_HOME/{app}/` (fallback `~/.local/state/{app}/`)
- Windows: `%LOCALAPPDATA%/{app}/logs/`

**Features:**
- JSON structured output via `slog`
- File rotation via lumberjack (size, age, backup count, compression)
- Multi-writer: stdout + file
- Sensitive field redaction — configured field names are replaced with `[REDACTED:N chars]`
- Source file/line in log entries

### [`events`](events/README.md) — Typed Event System

Type-safe wrapper for Wails v3 event emission. Keeps the kit Wails-version-agnostic via a `Backend` interface.

```go
import "github.com/jrschumacher/wails-kit/events"

// In your app setup, wrap the Wails app
emitter := events.New(events.BackendFunc(func(name string, data any) {
    app.EmitEvent(name, data)
}))

// Emit events
emitter.Emit(events.SettingsChanged, events.SettingsChangedPayload{
    Keys: []string{"appearance.theme"},
})

// For testing
mem := events.NewMemoryEmitter()
emitter := events.New(mem)
// ... trigger actions ...
mem.Events()  // all emitted events
mem.Last()    // most recent event
mem.Count()   // number of events
mem.Clear()   // reset
```

**Kit-provided event constants:**
- `events.SettingsChanged` (`"settings:changed"`)

Apps define their own event constants and payload types following the same pattern.

**Recommended frontend TypeScript pattern:**
```ts
export const Events = {
    SETTINGS_CHANGED: 'settings:changed',
} as const

export interface EventMap {
    [Events.SETTINGS_CHANGED]: { keys: string[] }
}
```

### [`updates`](updates/README.md) — GitHub-Based Auto-Updates

Self-update mechanism for desktop apps using GitHub Releases. Zero external dependencies — built on `net/http`, `encoding/json`, and an inline semver parser.

```go
import "github.com/jrschumacher/wails-kit/updates"

svc, err := updates.NewService(
    updates.WithCurrentVersion("v1.0.0"),
    updates.WithGitHubRepo("myorg", "myapp"),
    updates.WithEmitter(emitter),
    updates.WithAssetPattern("myapp_{os}_{arch}"),
)

// Check for updates (on startup, timer, or user action)
rel, err := svc.CheckForUpdate(ctx)  // emits updates:available
if rel != nil {
    svc.DownloadUpdate(ctx)   // emits updates:downloading → updates:ready
    svc.ApplyUpdate(ctx)      // replaces binary; app should prompt restart
}
```

**Features:**
- GitHub Releases API with optional auth token for private repos
- Semver comparison with full prerelease support
- Asset matching with OS/arch variants (darwin/macos, amd64/x86_64, etc.)
- Archive extraction (tar.gz, zip) with path traversal protection
- Progress events throttled to 250ms
- Settings group for check frequency, auto-download, prerelease opt-in

**Events:** `updates:available`, `updates:downloading`, `updates:ready`, `updates:error`

**Error codes:** `update_check`, `update_download`, `update_apply`

See [`updates/README.md`](updates/README.md) for full documentation.

### [`taskfiles`](taskfiles/README.md) — Shared Build & Release Tasks

Reusable [Task](https://taskfile.dev/) definitions for building, signing, and publishing wails-kit apps. Reads project config from a single `.wails-kit.yml` file.

```yaml
# .wails-kit.yml
app:
  name: MyApp
  bundle_id: com.example.myapp

release:
  github_repo: owner/myapp
  tap_github_repo: owner/homebrew-tap

signing:
  developer_id: "Developer ID Application: ..."
  keychain_profile: AC_PASSWORD
```

Include in your project's Taskfile:

```yaml
includes:
  release:
    taskfile: https://raw.githubusercontent.com/jrschumacher/wails-kit/main/taskfiles/release.yml
```

Then run:

```sh
task release              # uses latest GitHub release tag
task release VERSION=0.3.0  # explicit version
```

Handles macOS codesigning + notarization, archive creation, Homebrew tap upload, and cask updates. Signing is skipped gracefully when config values are empty.

**Requires:** [yq](https://github.com/mikefarah/yq) (`brew install yq`), [gh](https://cli.github.com/) (`brew install gh`)

See [`taskfiles/README.md`](taskfiles/README.md) for full documentation.

## Frontend Integration

The schema returned by `GetSchema()` is framework-agnostic JSON. Any frontend renders it with a generic loop:

```
for each group in schema.groups:
  render group heading
  for each field in group.fields:
    if field.advanced: group behind toggle
    if field.condition: check values[condition.field] in condition.equals
    if field.dynamicOptions: lookup options[values[dependsOn]]
    render input for field.type (text/password/select/toggle/number/computed)
```

### TypeScript Packages

| Package | Path | Description |
|---------|------|-------------|
| [`@wails-kit/types`](frontend/types/README.md) | `frontend/types` | TypeScript type definitions mirroring Go types |
| [`@wails-kit/settings`](frontend/settings/README.md) | `frontend/settings` | Headless settings logic: validation, conditions, dynamic options |

## Required Tools

| Tool | Install | Used by |
|------|---------|---------|
| [yq](https://github.com/mikefarah/yq) | `brew install yq` | Shared Taskfiles (reads `.wails-kit.yml`) |
| [gh](https://cli.github.com/) | `brew install gh` | Release upload, cask update |

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `ANTHROPIC_API_KEY` | Anthropic API key (used by SDK if no secret in settings) |
| `OPENAI_API_KEY` | OpenAI API key (used by SDK if no secret in settings) |
| `CF_AIG_AUTHORIZATION` | Cloudflare AI Gateway token |
| `{APP_PREFIX}_{FIELD_KEY}` | Keyring env var fallback for any password field (headless/CI) |

## Documentation

- [Architecture](docs/architecture.md) — package dependency graph, design patterns, adding new packages
- [Settings integration](docs/settings-integration.md) — how packages define and consume settings
