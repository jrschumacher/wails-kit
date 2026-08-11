// Package permissions is the check -> request -> "open System Settings"
// flow for OS-level permissions, macOS first: notifications, accessibility,
// and full disk access. These are the permissions that actually block
// features (a silently-failing notification, an accessibility API call
// that does nothing, a file read that returns EPERM), and every app
// rewrites the same brittle state machine to handle them.
//
// This is Wails-importing (AD-4 allowlist: shortcuts, windowstate,
// permissions, kit/wailsbridge) — the notifications Kind wraps
// wails/v3/pkg/services/notifications. It builds headlessly on every
// platform (no build tag) but Check/Request only report anything real
// against a live macOS session with a signed, bundled app; see the README
// and AGENTS.md for exactly what automated tests can and cannot cover.
//
// # Three states, not two
//
// Status is deliberately four-valued: granted, denied, not_determined, and
// unsupported. Collapsing "denied" and "not_determined" into one boolean is
// the classic mistake — they demand different UI. not_determined means
// "show the user why you're asking, then Request"; denied means "you
// cannot re-prompt, direct the user to OpenSystemSettings instead."
// unsupported means the Kind doesn't exist as a concept on this
// platform/build (a non-darwin GOOS, currently) and is never conflated
// with denied: an app should silently skip a feature on Linux, not tell
// the user they refused something they were never asked about.
//
// # The synthetic denied/not_determined split
//
// macOS itself does not always tell us which of "denied" or
// "not_determined" applies. The Wails notifications wrapper
// (CheckNotificationAuthorization) collapses the OS's richer
// UNAuthorizationStatus enum to a single granted/not-granted bool;
// AXIsProcessTrustedWithOptions for accessibility is boolean-only by
// design; and a Full Disk Access canary read reports EPERM identically
// whether the app was never listed in the FDA pane or was listed and
// explicitly turned off. For all three Kinds, Service therefore tracks —
// in memory, per process, never persisted — whether Request or
// OpenSystemSettings has already been called for a Kind ("seen"). A
// not-granted platform read reports Denied if the Kind has been seen this
// process, otherwise NotDetermined. This is an honest approximation, not
// an OS-reported fact: a fresh process restart re-reports NotDetermined
// even for a Kind the user explicitly denied in a previous run, until
// Request or OpenSystemSettings is called again. See AGENTS.md for the
// full rationale and the option to layer persistence on top.
package permissions

import (
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/i18n"
)

// Kind identifies an OS permission this package can check/request.
type Kind string

const (
	// Notifications wraps wails/v3/pkg/services/notifications'
	// CheckNotificationAuthorization / RequestNotificationAuthorization.
	Notifications Kind = "notifications"
	// Accessibility cannot be requested programmatically — Request opens
	// System Settings (via AXIsProcessTrustedWithOptions' own prompt
	// option) and returns whatever Check would return; there is no
	// programmatic grant.
	Accessibility Kind = "accessibility"
	// FullDiskAccess has no request API at all. Check probes a canary
	// path; Request is equivalent to OpenSystemSettings.
	FullDiskAccess Kind = "full_disk_access"
)

// Status is the resolved authorization state for a Kind. It is
// deliberately four-valued — see the package doc's "Three states, not
// two" section.
type Status string

const (
	// StatusGranted means the app currently holds the permission.
	StatusGranted Status = "granted"
	// StatusDenied means the user was asked (or sent to System Settings)
	// and the permission is not granted. The app must not re-prompt;
	// direct the user to OpenSystemSettings instead.
	StatusDenied Status = "denied"
	// StatusNotDetermined means the user has never been asked. Show a
	// rationale, then call Request.
	StatusNotDetermined Status = "not_determined"
	// StatusUnsupported means Kind is not a concept on this
	// platform/build. Never conflated with StatusDenied — an app should
	// skip the feature, not claim the user refused it.
	StatusUnsupported Status = "unsupported"
)

// Error codes for the permissions package.
const (
	ErrConfig       errors.Code = "permissions_config"
	ErrRequest      errors.Code = "permissions_request"
	ErrOpenSettings errors.Code = "permissions_open_settings"
	ErrUnsupported  errors.Code = "permissions_unsupported"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrConfig:       i18n.T("wailskit.permissions.errors.config", "Permissions is misconfigured. Please contact support."),
		ErrRequest:      i18n.T("wailskit.permissions.errors.request", "Failed to request permission. Please try again."),
		ErrOpenSettings: i18n.T("wailskit.permissions.errors.open_settings", "Failed to open System Settings. Please open it manually."),
		ErrUnsupported:  i18n.T("wailskit.permissions.errors.unsupported", "This permission isn't available on your operating system."),
	})
}

// errUnsupportedPlatform is returned by openSystemSettings implementations
// when a Kind has no settings pane to open on this platform (e.g. every
// Kind on non-darwin GOOS).
var errUnsupportedPlatform = errors.New(ErrUnsupported, "permissions: not supported on this platform", nil)

// EventChanged fires whenever Check or Request observes a Status
// transition for a Kind (never on every call — only when the resolved
// Status differs from the last one this Service observed for that Kind).
// Payload: ChangedPayload.
const EventChanged = "permissions:changed"

// ChangedPayload is EventChanged's payload.
type ChangedPayload struct {
	Kind   Kind   `json:"kind"`
	Status Status `json:"status"`
}
