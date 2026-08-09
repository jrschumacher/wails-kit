# permissions — agent notes

## Purpose

`permissions` owns the check -> request -> "denied, open System Settings"
state machine for OS-level permissions (macOS first: notifications,
accessibility, full disk access). It does not own notification content
(wraps `wails/v3/pkg/services/notifications` for the two auth calls only),
does not persist "has the user seen this before" across restarts
(deliberate — see Landmines), and does not own frontend rationale UI
(ships copy as `i18n.Text` constants; the app decides layout).

## Public API (load-bearing signatures)

```go
type Kind string
const (
    Notifications  Kind = "notifications"
    Accessibility  Kind = "accessibility"
    FullDiskAccess Kind = "full_disk_access"
)
type Status string // "granted" | "denied" | "not_determined" | "unsupported"

type Service struct{ /* unexported */ }
func NewService(opts ...Option) *Service // WithEmitter, WithLocalizer
func (s *Service) Check(k Kind) Status
func (s *Service) Request(ctx context.Context, k Kind) (Status, error)
func (s *Service) OpenSystemSettings(k Kind) error
func (s *Service) Text(t i18n.Text, args ...any) string // resolves copy.go constants
func (s *Service) Binding() *Binding                     // Check/Request/OpenSystemSettings for the webview

// copy.go: RationaleNotifications, DeniedNotifications, RationaleAccessibility,
// DeniedAccessibility, RationaleFullDiskAccess, DeniedFullDiskAccess, Unsupported
```

## Invariants (do not break)

- **`StatusUnsupported` is never conflated with `StatusDenied`.**
  `resolveAndEmit` (service.go) checks `!supported` first, before the seen
  map — even a "seen" `Kind` stays `Unsupported` forever if unsupported.
  `TestCheckUnsupportedNeverConflatedWithDenied` is the regression test.
  Breaking this tells a Linux user they refused something they were never
  asked about.
- **Never emit events while holding `s.mu`.** `resolveAndEmit` computes the
  status and releases the lock before `s.emitter.Emit` — three other kit
  packages shipped the deadlock version of this bug first.
- **`Request` never passes a context into `platform.request`.** Neither
  `RequestNotificationAuthorization` nor `AXIsProcessTrustedWithOptions`
  accept one. `Service.Request` layers cancellation on top by racing the
  platform call in a goroutine — do not "simplify" to a direct blocking
  call; that reintroduces the "may never return" bug.
- **`markSeen` fires unconditionally at the start of `Request`/
  `OpenSystemSettings`**, before the platform call runs — except when
  `ctx` is already cancelled on entry to `Request` (that early-return
  path never calls the platform or marks seen; see
  `TestRequestAlreadyCancelledContext`). Once the platform call *is*
  started, seen is recorded even if `ctx` cancels mid-flight or the call
  errors — the app did ask; that's what drives the denied/not_determined
  synthesis, not whether the answer arrived in time.
- **`platform` is the only OS-touching seam.** Every darwin-specific call
  (cgo, the notifications wrap, the FDA canary, `open`) lives behind the
  three `platform` methods (`platform.go`). `Service` imports no cgo and
  no `wails/v3` — only `platform_darwin.go` does.

## Dependencies & insulation

- `wails/v3/pkg/services/notifications` — on the AD-4 allowlist
  (`shortcuts`, `windowstate`, `permissions`, `kit/wailsbridge`). Imported
  only in `platform_darwin.go`, tagged `//go:build darwin` (no legacy
  `wails` tag). `notifications.New()` is a process-wide singleton this
  package calls into, never constructs itself.
- cgo (`ApplicationServices`, `Foundation`) for `AXIsProcessTrustedWithOptions`
  and the bundle-validity guard — inline C in the preamble, no separate `.c`/`.h`.
- `errors`, `events`, `i18n` (kit packages) — standard AD-9 pattern.

## Extension points

- A new `Kind`: a const here, a case in all three `platform` methods on
  every platform file, rationale/denied `i18n.Text` constants in
  `copy.go` (+ `locales/en.json`), a row in README's per-`Kind` table.
- A new platform (e.g. real Windows notification authorization): a new
  `//go:build windows` file implementing `platform`; narrow
  `platform_other.go`'s tag to `!darwin && !windows`.
- Persisting "seen" across restarts (out of scope — see Landmines) would
  be a `WithStore` option threading a `state.Store` through, following
  `windowstate`'s pattern. Keep the in-memory default so a caller who does
  nothing gets today's honest behavior.

## Testing

- `fakePlatform` (`fake_test.go`) implements `platform` with per-`Kind`
  `granted`/`supported` maps and an optional `requestBlock` channel that
  hangs `request` until the test closes it — makes
  `TestRequestContextCancelledMidFlight` deterministic, not racy.
- `go test -race ./permissions/` must cover: all `Status` outcomes from
  `Check`, `Unsupported` surviving `Request` (never becoming `Denied`),
  an already-cancelled and a mid-flight-cancelled `Request`,
  `OpenSystemSettings` marking seen, transition-only `EventChanged`,
  `Text`'s no-localizer fallback, `Binding` delegation.
- **Not automatable here**: an actual OS grant/deny cycle. Needs a live
  macOS session, a signed+bundled app (unbundled runs report Notifications
  as `StatusUnsupported` — see Landmines — rather than testing anything
  real), and a human clicking through System Settings. Manual checklist:
  build+sign `examples/permissions/`, run it, verify (a) first `Check`
  reports `NotDetermined` for all three kinds, (b) `Request(Notifications)`
  shows the native dialog and `Check` afterward matches what you clicked,
  (c) `OpenSystemSettings` per `Kind` opens the *correct* pane (the
  `x-apple.systempreferences:` URLs are undocumented Apple URLs, not a
  supported API; re-verify after macOS upgrades).

## File map

- `permissions.go` — `Kind`, `Status`, error codes + `RegisterMessages`,
  events, package doc (read first — full "denied vs not_determined" rationale).
- `copy.go` — exported `i18n.Text` prompt-copy constants.
- `platform.go` — the `platform` interface (the test seam).
- `service.go` — `Service`, `Option`/`config`, `Check`/`Request`/
  `OpenSystemSettings`/`Text`, `markSeen`, `resolveAndEmit`.
- `binding.go` — `Binding`, the Wails-registrable surface.
- `platform_darwin.go` — cgo `AXIsProcessTrustedWithOptions` wrapper,
  bundle-validity guard, notifications wrap, FDA canary, `open` URLs.
- `platform_other.go` — `!darwin`: everything `StatusUnsupported`.
- `fake_test.go`, `service_test.go` — the test double and suite.
- `locales/en.json` — catalog for every key; `TestCatalogKeysExist` (i18n
  package) fails on a missing one.

## Landmines

- **A cancelled `Request`'s goroutine keeps running.** `platform.request`
  can't be cancelled (the macOS APIs accept no context), so a ctx-timeout
  `Request` returns promptly but leaves the real call running until it
  resolves itself (`RequestNotificationAuthorization` has its own 180s
  internal timeout; accessibility returns near-instantly regardless). A
  deliberate, bounded leak — not a bug to "fix" with unsupported cancellation.
- **denied/not_determined does not survive a process restart** unless the
  app persists it. Don't add hidden persistence inside `Service` — see
  Extension points; the alternative is inventing OS state this package
  cannot verify.
- **`fullDiskAccessGranted`'s canary is `~/Library/Safari`** — TCC-protected
  regardless of probing app, present on every macOS install. If Apple ever
  changes this, it needs a new canary — verify with a fresh `os.ReadDir`
  test on the target macOS version before changing it.
- **Calling into `UNUserNotificationCenter` without a real app bundle is a
  process-killing `SIGABRT`, not a Go error** — contrary to what the
  roadmap this package implements assumed. It raises an uncaught
  `NSInternalInconsistencyException`, unrecoverable from Go.
  `hasValidMainBundle()` (cgo, `platform_darwin.go`) guards every call
  into `notifications.New()`; never remove it or add a new SDK call site
  without it (confirmed: `go run ./examples/permissions` without the guard
  crashes instantly).
