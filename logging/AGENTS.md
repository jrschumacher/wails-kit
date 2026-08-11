# logging — agent notes

## Purpose

`logging` provides OS-aware, file-rotated, JSON structured logging on top of
`slog`, with sensitive-field redaction. It owns log destination (file +
optional stdout), rotation policy, and redaction — not what apps choose to
log (redacting log *content*, e.g. scrubbing an LLM prompt string itself
rather than a whole attr value, stays the app's job; see README "Sensitive
field redaction").

Unlike most kit packages this is a `Config` struct + package-level
`Init`/`Get` singleton, not `NewService(opts ...Option)`. That shape is
deliberate, not legacy debt: AD-7 (`docs/v2-roadmap.md`) explicitly keeps
`logging`'s consumer-facing API "unchanged" across the v2 migration, and
`kit.New` (WP-30, not yet built) wants `slog.SetDefault`-style global
ergonomics — one `Init` call early in `main`, then any code anywhere calls
`logging.Info(...)` without threading a `*Logger` through every function
signature. Do not refactor this to functional options; if that tradeoff
ever needs revisiting, it is a roadmap decision, not a package-local one.

## Public API (load-bearing signatures)

```go
type Config struct {
    AppName       string   // default: "app"
    Level         string   // "debug"|"info"|"warn"|"error", default: "info"
    LogDir        string   // override; default: appdirs.New(AppName).Log()
    MaxSize       int      // MB, default: 100
    MaxAge        int      // days, default: 7
    MaxBackups    int      // default: 10
    Compress      *bool    // default: true (nil resolves to true)
    AddSource     bool     // default: false
    SensitiveKeys []string // attr keys to redact, at any slog.Group depth
    Stdout        *bool    // default: true (nil resolves to true)
}

func Init(config *Config) error // safe to call repeatedly; replaces the global logger
func Get() *Logger              // lazy-inits with defaults if Init was never called

func Debug(msg string, args ...any)
func Info(msg string, args ...any)
func Warn(msg string, args ...any)
func Error(msg string, err error, args ...any)

func (l *Logger) WithFields(args ...any) *Logger
func NewRedactingHandler(inner slog.Handler, sensitiveKeys []string) *RedactingHandler
```

## Invariants (do not break)

- **`Compress` and `Stdout` are `*bool`, not `bool`.** This is the fix for
  the exact defect that motivated this file: a plain `bool` field can't
  distinguish "caller didn't set this" from "caller explicitly set false,"
  so a documented default of `true` silently became `false` because the
  zero value always won. `resolveDefaults` in `logger.go` is the single
  place that interprets `nil`. If you add another defaulted boolean config
  field, use the same pointer pattern and extend `resolveDefaults` — don't
  reintroduce a plain `bool` default gap.
- **Redaction marker is the fixed string `[REDACTED]`, never anything that
  encodes the secret's length** (e.g. `[REDACTED:9 chars]`). Length alone
  narrows a brute-force search. `TestRedactingHandler_DoesNotLeakLength`
  enforces this; don't relax it even if a future README draft asks for a
  "helpful" length hint.
- **Redaction recurses into `slog.Group`.** `redactAttr` in `redact.go`
  checks `resolved.Kind() == slog.KindGroup` *before* checking the
  sensitive-key set, and recurses into every member attr. Groups
  (`slog.Group("session", "token", tok)`) are a normal slog idiom; if
  redaction only inspected top-level keys again, any attr nested in a
  group would silently bypass it — which is exactly the bug this file
  exists to prevent from recurring. `TestRedactGroupRecursion` and
  `TestRedactGroupRecursion_Nested` are the regression tests.
- **Log directory is created `0700`, not `0755`.** Logs are user content —
  for a consumer like Prune they carry LLM prompt/response traffic — so the
  directory follows the kit's `0700` convention (matches `appdirs`,
  `state`, `database`) rather than a world-readable default.
- **`Init`/`Get` must stay race-safe.** `loggerMu` guards `defaultLogger`;
  `initOnce` guards the lazy-init path in `Get`. `TestInit_RaceCondition`
  hammers concurrent `Init`/`Get` under `-race` — preserve that shape if
  you touch either function.

## Dependencies & insulation

- `appdirs` — resolves the OS-standard log directory when `LogDir` is
  unset.
- `errors` (kit package) — `Init`'s failure path returns an
  `errors.Code`-tagged `*errors.UserError` (`ErrLoggingInit`), registered
  via `RegisterMessages` in `init()`. Do not reintroduce bare
  `fmt.Errorf` for user-facing failures here; `fmt.Fprintf(os.Stderr, ...)`
  remains for the internal "the handler itself failed" fallback in
  `logWithCaller`, which has no user-facing error path to return through.
- `gopkg.in/natefinch/lumberjack.v2` — the only rotation implementation.
  Note the correct spelling is `natefinch` (README previously linked the
  typo'd `natefinsh`, which 404s).
- No `wails/v3` import; this package is Wails-free per the AD-4 allowlist
  and must stay that way — Prune's CLI/TUI processes call `logging.Init`
  directly with no `application.App` in the process.

## Extension points

- New `Config` fields with a default that isn't the zero value: use the
  `*bool`/pointer pattern above, resolved in `resolveDefaults`.
- New redaction shapes beyond flat key-match: extend `redactAttr` in
  `redact.go`. Keep the recursion structure — resolve the value first,
  branch on `Kind() == slog.KindGroup` before the sensitive-key check.
- A structured (non-JSON) output mode would be a new handler wrapping the
  same `RedactingHandler`, not a change to `RedactingHandler` itself, which
  should stay format-agnostic (it operates on `slog.Attr`, not bytes).

## Testing

- No test doubles needed beyond `t.TempDir()` for `LogDir` and
  `bytes.Buffer` + `slog.NewJSONHandler` for handler-level redaction tests
  (see existing `TestRedactingHandler*`).
- `go test -race ./logging/` must cover: group-recursive redaction
  (`TestRedactGroupRecursion`, `TestRedactGroupRecursion_Nested`), the
  `Compress`/`Stdout` default resolution (`TestCompressDefaultTrue`,
  `TestStdoutDefaultTrue`, and their explicit-false counterparts), and the
  `0700` log directory permission (`TestInit_LogDirIs0700`).
- Every test that calls `Init` must reset package state afterward:
  `loggerMu.Lock(); defaultLogger = nil; loggerMu.Unlock(); initOnce =
  syncOnce()` — see existing tests for the pattern. Skipping this leaks
  state into later tests in the same run.
- Not automatable: real OS log-directory conventions per platform (that's
  `appdirs`'s test surface, not this package's).

## File map

- `logger.go` — `Config`, `resolvedConfig`/`resolveDefaults` (pure
  defaulting logic, unit-testable without touching the filesystem or
  `os.Stdout`), `Init`, `Get`, `Logger`, package-level
  `Debug`/`Info`/`Warn`/`Error`, `parseLevel`.
- `redact.go` — `RedactingHandler`, the group-recursive `redactAttr`.
- `logger_test.go` — all tests for both files above.
- `README.md` — quickstart, config table with defaults, redaction
  semantics (including the group-recursion example), rotation behavior.

## Landmines

- `resolveDefaults(nil)` is valid, identical to `resolveDefaults(&Config{})`
  — both resolve `AppName` to `"app"`. Don't add a nil-config special case
  elsewhere that diverges from it.
- `Get()`'s lazy-init path calls `Init(&Config{AppName: "app"})` — it does
  **not** set `Stdout`, relying on `resolveDefaults` to resolve the unset
  pointer to `true`. If `Stdout: true` (a bool literal) reappears anywhere,
  that's the pre-fix regression — `Stdout` is `*bool`.
- `logWithCaller` hardcodes `runtime.Callers(3, pcs[:])` to attribute the
  correct source line to `Logger.Error`. Another wrapping layer means the
  skip count needs updating or `AddSource` output points at the wrong
  frame.
