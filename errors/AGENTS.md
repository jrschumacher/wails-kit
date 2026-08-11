# errors — agent notes

## Purpose

`errors` is the kit's structured, user-facing error type: a `Code` (stable, wire-level
identifier) paired with a technical `Message` (for logs) and a translatable user message
(`i18n.Text`, resolved to a plain string on demand). It owns: `UserError`, the code
registry (`RegisterMessages`/`getUserText`), and read-time localization of the
user-facing text (`SetLocalizer`/`GetUserMessage`). It does **not** own: choosing *which*
locale is active (that's `i18n.Localizer`), deciding what a code means to a frontend
(that's the consuming app — e.g. Prune's `app/frontend/src/lib/errors.ts`), or any
business logic tied to a specific code.

## Public API (load-bearing signatures)

```go
type Code string

type UserError struct {
    Code       Code
    Message    string
    UserMsg    string    // construction-time English snapshot — see Landmines
    Text       i18n.Text // translatable message
    Underlying error
    Fields     map[string]any
}

func New(code Code, message string, underlying error) *UserError
func Newf(code Code, format string, args ...any) *UserError // %w wraps Underlying, exactly like fmt.Errorf; %v/%s do not
func Wrap(code Code, message string, err error) *UserError

func GetUserMessage(err error) string // resolves live via the installed localizer, or Text.Other
func GetCode(err error) Code
func IsCode(err error, code Code) bool

func RegisterMessages(msgs map[Code]i18n.Text)
func SetLocalizer(l *i18n.Localizer) // nil clears it; package-level, not per-error
```

## Invariants (do not break)

- **Codes never change string value when text is localized.** `Code` is a wire contract
  (Prune's frontend branches on the literal string). A WP that touches this package may
  add codes; it must never rename or repurpose an existing one. `TestCodesUnchanged`
  pins the current set.
- **`New`/`Newf`/`Wrap` never touch the localizer.** They resolve `Text` from the
  registry (`getUserText`) and snapshot `Text.Other` into `UserMsg` — nothing here calls
  `resolveText`/`SetLocalizer`'s installed localizer at construction time. This is what
  makes it safe to construct (and register messages for) errors in package `init()`,
  which runs before any app has wired up i18n.
- **Resolution against the installed localizer happens only at read time** —
  `GetUserMessage` and `UserError.MarshalJSON`. Nothing caches a resolved string across
  a `SetLocale` call; the next read reflects it.
- **Nil localizer → `Text.Other`, always.** `resolveText` is the single choke point for
  this; don't reimplement the nil check elsewhere. This is the "no localizer still gets
  sensible English" guarantee — localization is opt-in throughout the kit.
- **`Newf` only extracts `Underlying` for an actual `%w` verb**, via
  `stderrors.Unwrap(fmt.Errorf(format, args...))` — never by scanning `args` for the
  first value that implements `error`. (This was a real bug: the old implementation set
  `Underlying` for `%v`/`%s` too, contradicting its own doc comment. `TestNewf_NoWrapWithoutWVerb`
  pins the fix.)

## Dependencies & insulation

- **`errors` imports `i18n`, one-way.** `i18n` must never import `errors` — that would
  recreate the exact shape of cycle WP-12 broke for `settings` (`i18n → settings →
  i18n`). `errors` is lower-level than `settings`: it has no schema, no groups, nothing
  `i18n` would ever need back from it. If a future change makes `i18n` want something
  from `errors`, that is a design smell — push the shared piece down into a new leaf
  package both import, don't loop the edge back.
- No `wails/v3` import, and none should ever be added — `errors` is used from Prune's
  CLI/TUI as well as the GUI (AD-4 allowlist: this package stays on the Wails-free side).
- `events`: not used. Errors are returned, not emitted; if a future WP wants an
  "error occurred" event, that belongs to the *emitting* package (e.g. `updates` already
  has `EventError`), not to this one.

## Extension points

- New built-in codes: add the `Code` const, a `defaultMessages` entry
  (`i18n.T("wailskit.errors.<code>", "...")`), and the matching key in
  `errors/locales/en.json`. Run `go test ./i18n/ -run TestCatalogKeysExist` — it fails
  loudly if the catalog entry is missing.
- Apps/other kit packages register their own codes via `RegisterMessages` in their own
  `init()`, with their own `i18n.Text` keys (their own namespace, not `wailskit.`) and
  their own `locales/en.json` next to their source.

## Testing

- No test doubles needed for the non-i18n surface — `stderrors.New` plain errors are
  enough to exercise `Unwrap`/`Is`/`As` paths.
- i18n tests build a real `*i18n.Localizer` via `i18n.New(i18n.WithCatalog(fstest.MapFS{...}), i18n.WithLocale(...))`
  rather than a mock — the package has no seam for a fake localizer, and it's a small
  concrete type, so a real one is simpler than an interface+double.
- **Every i18n test must `t.Cleanup(func() { SetLocalizer(nil) })`.** The installed
  localizer is process-global (package-level `var`), so a test that sets it and forgets
  to clear it will leak into every test that runs after it in the same `go test`
  invocation — including ones in this file that assert the nil-localizer fallback.
- `go test -race ./errors/` must cover: construction (`New`/`Newf`/`Wrap`), the `%w` vs
  `%v`/`%s` extraction fix, code/message extraction helpers, `RegisterMessages`
  override behavior, nil-localizer fallback, read-time resolution (constructed before
  `SetLocalizer`, correct after), catalog resolution, locale switching, `MarshalJSON`'s
  live resolution, and code-value stability.

## File map

- `errors.go` — `Code`, `UserError` (+`MarshalJSON`), `New`/`Newf`/`Wrap`,
  `GetUserMessage`/`GetCode`/`IsCode`, the message registry (`RegisterMessages`,
  `getUserText`, `defaultMessages`), and the localizer wiring (`SetLocalizer`,
  `resolveText`).
- `locales/en.json` — this package's own catalog, keyed `wailskit.errors.<code>`.

## Landmines

- **`UserMsg` (the field) and `GetUserMessage(err)` (the function) answer different
  questions.** The field is frozen at construction, always English. The function
  re-resolves every call against whatever localizer is installed *now*. Don't "simplify"
  by reading `.UserMsg` where live resolution is wanted — `updates/service.go`'s
  `errors.New(code, "", nil).UserMsg` is an existing internal use that deliberately
  wants the always-English snapshot (it's building an English log-adjacent payload, not
  user-facing UI text); don't treat it as a template for new code that *does* want
  localization.
- **`messages` and `localizer` are separate package-level globals, guarded by separate
  mutexes (`msgMu`, `locMu`).** `RegisterMessages` only ever touches `messages`;
  `SetLocalizer` only ever touches `localizer`. Don't merge them into one lock — nothing
  requires their updates to be atomic with each other, and the extra lock scope would be
  wasted contention.
- `getUserText` **never** calls the localizer — resolving `Text.Other` too early there
  (e.g. "just call `resolveText` once and cache the string") would silently reintroduce
  construction-time resolution and break `TestResolveAtReadTime`.
