// Package health is the kit's single connectivity registry. Apps and other
// kit packages register endpoints; the registry probes them on a cadence
// (or on demand), caches the last-known state per endpoint, and exposes a
// resolved snapshot plus change events.
//
// The reason this is a package and not a helper function is that it
// distinguishes three failure modes with different remedies:
//
//   - ClassConnectivity: there is no network at all (remedy: nothing an app
//     can do — tell the user, don't blame a backend).
//   - ClassBackend: your own API is down (remedy: status page, retry).
//   - ClassProvider: a third party is down (remedy: different messaging —
//     it isn't the app's fault).
//
// A frontend `navigator.onLine` check collapses all three into one
// frequently-wrong answer (it reports true when connected to a router with
// no upstream). One registry knows reachability; every other kit package —
// and the app — subscribes rather than embedding its own probe. See
// docs/v2-roadmap.md WP-15.
package health

import (
	"context"
	"time"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/i18n"
)

// Class classifies what a failing check means for the user-facing remedy.
type Class string

const (
	// ClassConnectivity means "is there a network at all". A down
	// connectivity check suppresses backend/provider checks to StateUnknown
	// (see Registry.Snapshot) rather than letting them claim StateDown.
	ClassConnectivity Class = "connectivity"
	// ClassBackend means the app's own API. Remedy: status page, retry.
	ClassBackend Class = "backend"
	// ClassProvider means a third-party dependency. Remedy: different
	// messaging — it isn't the app's fault.
	ClassProvider Class = "provider"
)

// State is a check's (or the registry's overall) resolved health.
type State string

const (
	// StateUnknown means "not yet probed" — distinct from StateHealthy.
	// A freshly Register-ed check starts here, not at StateHealthy; a
	// snapshot that reports an unprobed endpoint as healthy would drive a
	// green badge for something nobody has actually checked.
	StateUnknown State = "unknown"
	// StateHealthy means the most recent probe succeeded.
	StateHealthy State = "healthy"
	// StateDegraded is reserved for probe implementations that want to
	// signal a non-fatal issue (e.g. elevated latency) without reporting a
	// hard failure. Nothing in this package's own probes produces it —
	// Probe.Probe returns only success/error — but Snapshot's suppression
	// and Overall-computation logic honor it if a custom Probe's caller
	// sets a Check's status via a future extension.
	StateDegraded State = "degraded"
	// StateDown means the most recent probe failed.
	StateDown State = "down"
)

// severity ranks State from least to most severe, used to compute
// Snapshot.Overall as "the worst state among critical checks". Unknown
// ranks above Healthy (worse) — an unprobed critical endpoint must not be
// able to make Overall look healthier than it honestly is.
var severity = map[State]int{
	StateHealthy:  0,
	StateUnknown:  1,
	StateDegraded: 2,
	StateDown:     3,
}

func worseState(a, b State) State {
	if severity[b] > severity[a] {
		return b
	}
	return a
}

// Probe is anything that can check reachability once. Implementations must
// respect ctx cancellation and should apply their own per-request timeout
// (see HTTPProbe) — a probe that ignores ctx and blocks forever will leak
// the goroutine running it and can prevent Registry.Start from shutting
// down promptly.
type Probe interface {
	Probe(ctx context.Context) error
}

// ProbeFunc adapts a plain function to Probe.
type ProbeFunc func(ctx context.Context) error

// Probe calls f.
func (f ProbeFunc) Probe(ctx context.Context) error { return f(ctx) }

// Check describes one registered endpoint.
type Check struct {
	// Name must be unique within a Registry; Register errors on duplicates.
	Name string
	// Class determines how a failure is presented (see Class).
	Class Class
	// Critical marks this check as participating in Snapshot.Overall. A
	// non-critical check still probes, still reports its own State in
	// Snapshot.Checks, and still emits health:changed on transition — it
	// just never drags Overall down.
	Critical bool
	// Interval is this check's own probing cadence. Zero means "use the
	// registry's default interval" (see WithDefaultInterval).
	Interval time.Duration
	// Probe performs the actual reachability test.
	Probe Probe
}

// CheckStatus is one check's last-known, resolved state.
type CheckStatus struct {
	Name      string        `json:"name"`
	Class     Class         `json:"class"`
	State     State         `json:"state"`
	Critical  bool          `json:"critical"`
	Err       string        `json:"err,omitempty"`
	CheckedAt time.Time     `json:"checkedAt"`
	Latency   time.Duration `json:"latency"`
}

// Snapshot is the registry's resolved, point-in-time view — what a status
// page or badge renders.
type Snapshot struct {
	// Overall is the worst State among critical checks, after the
	// offline-suppression rule below. A registry with no critical checks
	// (including one with none registered at all) reports StateUnknown —
	// never StateHealthy for "nothing has been checked".
	Overall State `json:"overall"`
	// Offline is true when any ClassConnectivity check is StateDown.
	Offline bool `json:"offline"`
	// Checks is every registered check's status, in registration order.
	// When Offline is true, every non-connectivity check's State is
	// reported as StateUnknown here (not StateDown) — "your API is down"
	// must never be claimed when the real problem is "there is no network
	// at all". The underlying, unsuppressed state is not discarded: it
	// still drove the health:changed event when it transitioned, and the
	// next successful probe after connectivity returns will report it
	// again normally.
	Checks []CheckStatus `json:"checks"`
}

// Event names and payloads.
const (
	// EventChanged is emitted only when a check's State actually changes
	// (never on every probe — a healthy endpoint pinging every 30s does
	// not spam the frontend).
	EventChanged = "health:changed"
)

// ChangedPayload is EventChanged's payload.
type ChangedPayload struct {
	Check   CheckStatus `json:"check"`
	Overall State       `json:"overall"`
}

// Error codes.
const (
	ErrDuplicateCheck errors.Code = "health_duplicate_check"
	ErrInvalidCheck   errors.Code = "health_invalid_check"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrDuplicateCheck: i18n.T("wailskit.health.errors.duplicate_check", "This health check is already registered."),
		ErrInvalidCheck:   i18n.T("wailskit.health.errors.invalid_check", "This health check is missing required configuration."),
	})
}
