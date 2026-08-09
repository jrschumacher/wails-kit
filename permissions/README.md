# permissions

Package `permissions` is the check -> request -> "denied, open System
Settings" flow for OS-level permissions, macOS first: notifications,
accessibility, and full disk access. These are the permissions that
actually block features — a notification that silently never appears, an
accessibility API call that silently does nothing, a file read that
silently returns EPERM — and every app rewrites the same brittle state
machine to handle them, usually collapsing "denied" and "not yet asked"
into one boolean along the way.

## Usage

```go
import "github.com/jrschumacher/wails-kit/v2/permissions"

svc := permissions.NewService()

switch svc.Check(permissions.Notifications) {
case permissions.StatusGranted:
    // send notifications freely
case permissions.StatusNotDetermined:
    // show svc.Text(permissions.RationaleNotifications), then:
    status, err := svc.Request(ctx, permissions.Notifications)
    // status is Granted or Denied (or NotDetermined again if ctx expired
    // before the OS dialog resolved — see "Request and context" below)
case permissions.StatusDenied:
    // cannot re-prompt — show svc.Text(permissions.DeniedNotifications)
    // with a button that calls:
    _ = svc.OpenSystemSettings(permissions.Notifications)
case permissions.StatusUnsupported:
    // not a concept on this platform/build — skip the feature silently
}
```

Register `svc.Binding()` (never `svc` itself — see "Binding, not Service"
below) with Wails for the same three calls from the frontend:

```go
application.NewService(svc.Binding())
```

## The three (four) states

`Status` is `granted | denied | not_determined | unsupported`. Three
states are the documented minimum in the roadmap this package implements
(`docs/v2-roadmap.md`, WP-21) because `not_determined` and `denied` demand
different UI:

- **`not_determined`** — the user has never been asked. Show a rationale
  (`svc.Text(permissions.RationaleNotifications)` or the equivalent for
  another `Kind`), then call `Request`.
- **`denied`** — the user was asked (or sent to System Settings) and said
  no, or turned it off. **You cannot re-prompt.** Show
  `svc.Text(permissions.DeniedNotifications)` (or the `Kind`-appropriate
  variant) with a button that calls `OpenSystemSettings`.
- **`granted`** — use the feature.
- **`unsupported`** — `Kind` isn't a concept on this platform/build.
  **Never treated as `denied`** — an app should silently skip the feature
  on Linux, not tell the user they refused something they were never
  asked about.

## Where `denied` vs `not_determined` actually comes from

This is the part every consumer should understand before relying on it:
**macOS itself does not always report which of the two applies**, and this
package cannot invent OS-level information that doesn't exist.

- Wails' `notifications.CheckNotificationAuthorization` collapses the OS's
  richer `UNAuthorizationStatus` enum (`notDetermined` / `denied` /
  `authorized` / `provisional` / `ephemeral`) to a single
  granted-or-not `bool`.
- `AXIsProcessTrustedWithOptions` (accessibility) is boolean-only by
  design — there is no "not determined" state exposed at all.
- A Full Disk Access canary read returns the same `EPERM` whether the app
  was never listed in the FDA pane or was listed and explicitly turned
  off.

So `Service` tracks, **in memory, per process, never persisted to disk**,
whether `Request` or `OpenSystemSettings` has already been called for a
`Kind` ("seen"). A not-granted platform read reports `Denied` if the
`Kind` has been seen this process, otherwise `NotDetermined`. This is an
honest, documented approximation — not an OS-reported fact. **A fresh
process restart re-reports `NotDetermined`** even for a `Kind` the user
explicitly denied in a previous run, until `Request` or
`OpenSystemSettings` is called again in the new process. An app that
needs the distinction to survive restarts must persist it itself (e.g.
record "I've asked about X" in its own settings) — this package
deliberately doesn't do that for you, to avoid inventing a false sense of
certainty about state it cannot actually observe.

## Per-`Kind` behavior (macOS)

| `Kind` | `Check` | `Request` |
|---|---|---|
| `Notifications` | Wraps `wails/v3/pkg/services/notifications`' `CheckNotificationAuthorization`. | Wraps `RequestNotificationAuthorization` — shows the native OS dialog. Requires a real application bundle; see the crash warning below. |
| `Accessibility` | `AXIsProcessTrustedWithOptions(prompt: false)` via cgo. | **Cannot be requested programmatically.** Calling `Request` passes `prompt: true`, which asks macOS to show its own trust dialog and list the app in System Settings — but the call returns immediately with the pre-decision trust state; it does not wait for the user. Pair `StatusNotDetermined`/`StatusDenied` with `OpenSystemSettings`. |
| `FullDiskAccess` | Reads `~/Library/Safari` as a canary — success means granted, any error means not granted. There is no API for this permission at all. | **No request API exists.** `Request` is exactly `OpenSystemSettings` followed by a fresh `Check`. |

**A correction to the roadmap this package implements (WP-21):** the
roadmap's design notes assumed an unbundled/unsigned dev build would make
`CheckNotificationAuthorization`/`RequestNotificationAuthorization` "report
an SDK error" — a normal Go `error` return. That's not what actually
happens. `UNUserNotificationCenter.currentNotificationCenter` (which those
calls reach into) raises an **uncaught Objective-C exception**
(`NSInternalInconsistencyException: bundleProxyForCurrentProcess is nil`)
and hard-**aborts the whole process** (`SIGABRT`) when the running binary
has no real `NSBundle` identifier — exactly the case for `go run`,
`go test`, and any unsigned dev build. There is no Go-level `recover` for
an uncaught Objective-C exception. `platform_darwin.go` therefore checks
`hasValidMainBundle()` (a small cgo `NSBundle.mainBundle.bundleIdentifier`
check) before ever touching the notifications SDK, and reports
`StatusUnsupported` instead of crashing when it's false. If you're
extending this package: never call `notifications.New()`'s methods without
that guard in front of them.

Non-darwin `GOOS` reports `StatusUnsupported` for every `Kind` — see
`platform_other.go`. The package builds and its tests pass on every
platform (`GOOS=linux go build ./permissions/...` is part of CI); only the
darwin build tag pulls in cgo and the Wails notifications SDK.

## `Request` and context

Requesting is asynchronous by nature — the user can leave an OS dialog
open indefinitely, and `AXIsProcessTrustedWithOptions`/
`RequestNotificationAuthorization` accept no `context.Context` of their
own to cancel with. `Service.Request(ctx, k)` races the underlying
platform call in a goroutine against `ctx`: if `ctx` is done first,
`Request` returns immediately with `Check(k)`'s current answer and
`ctx.Err()` — it never blocks past your deadline. The platform call itself
keeps running in the background (it cannot be cancelled) until it resolves
on its own; this is a documented, not silently hidden, limitation — see
AGENTS.md.

Calling `Request` at all — regardless of outcome, and even if `ctx`
expires before the platform call returns — marks the `Kind` "seen": the
app has now asked, so a subsequent not-granted `Check` reports `Denied`,
never `NotDetermined` again this process.

## Binding, not Service

Wails v3 binds every exported method of a registered service. Register
only `svc.Binding()` — never `*Service` directly — the same rule
`settings.Binding` and `health.Binding` follow. `Binding.Request` calls
`Service.Request` with `context.Background()`: a webview RPC call has no
mechanism to hand it a cancellable Go context, so a frontend that wants a
"stop waiting" UI does so by not awaiting the promise and re-checking with
`Check` later, not by cancellation.

## Prompt copy

The rationale ("why we're asking") and denied ("here's the fix") strings
ship as exported `i18n.Text` constants (`copy.go`): `RationaleNotifications`
/ `DeniedNotifications`, `RationaleAccessibility` / `DeniedAccessibility`,
`RationaleFullDiskAccess` / `DeniedFullDiskAccess`, plus a generic
`Unsupported`. Resolve them Go-side with `Service.Text` (falls back to the
literal English `Other` value when no `WithLocalizer` is configured); a
webview frontend instead reads the equivalent `wailskit.permissions.*` keys
out of the app's merged i18n catalog (`i18n.Binding.GetCatalog`) directly —
these are static per-`Kind` strings, not something `Binding` needs its own
method to serve.

## Options

| Option | Description |
|---|---|
| `WithEmitter(e)` | Optional `*events.Emitter`. Fires `permissions:changed` on `Status` transitions. |
| `WithLocalizer(l)` | Optional `*i18n.Localizer` used by `Service.Text`. Without it, `Text` returns each constant's literal `Other` (English) value. |

## Events

| Event | Payload | Description |
|---|---|---|
| `permissions:changed` | `ChangedPayload{Kind, Status}` | Fired by `Check` or `Request` when the resolved `Status` for a `Kind` differs from the last one this `Service` observed — transition-only, not on every call. |

## Error codes

| Code | User message |
|---|---|
| `permissions_config` | Permissions is misconfigured. Please contact support. |
| `permissions_request` | Failed to request permission. Please try again. (wraps a platform-reported request error, e.g. the notifications SDK call failing) |
| `permissions_open_settings` | Failed to open System Settings. Please open it manually. (the `open` command itself failed) |
| `permissions_unsupported` | This permission isn't available on your operating system. |

## What automated tests cover, and what they can't

`go test -race ./permissions/` exercises the entire state machine —
`Check`/`Request`/`OpenSystemSettings`, the denied/not_determined
synthesis (including that `Unsupported` is never conflated with `Denied`
even after `Request` marks a `Kind` "seen"), transition-only event
emission, and `Request`'s context-cancellation race (including the
"platform call blocks indefinitely, `Request` must still return promptly"
case) — against a `fakePlatform` (`fake_test.go`) that never touches cgo,
a real display, or an OS permission prompt.

**Not automatable here**: an actual grant/deny cycle needs a live macOS
session with a signed, bundled app — `CheckNotificationAuthorization`
reports an SDK error (surfaced as `StatusUnsupported`) against an unsigned
dev build, `AXIsProcessTrustedWithOptions` and the Full Disk Access canary
both require a real logged-in user to have made a real decision in System
Settings, and the `x-apple.systempreferences:` deep-link URLs in
`platform_darwin.go` can only be confirmed to open the right pane by
actually clicking through them — Apple has not guaranteed they survive
future macOS releases. See AGENTS.md's manual verification checklist.
