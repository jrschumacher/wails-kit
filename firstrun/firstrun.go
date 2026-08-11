// Package firstrun detects, on each app launch, whether this is a fresh
// install, an upgrade from a previously-recorded version, a downgrade, or
// an unchanged relaunch — and runs ordered hooks in response. Every app
// that ships more than once needs this to run data migrations, show a
// what's-new screen, or refuse to open data written by a newer version;
// this package exists so each app does not hand-roll the same ~80 lines
// and get the ordering wrong.
//
// # Version stamp
//
// The last version a successful Run completed for is persisted via
// state.Store in the app's data directory. Detect reads that stamp and
// classifies the transition; Run additionally executes matching Hooks and,
// only if every one of them succeeds, updates the stamp. A failed hook
// leaves the stamp untouched, so the next launch retries the same set of
// hooks — hooks must therefore be idempotent (see the Hook docs).
//
// # Adopted mid-life
//
// An app that adopts this package after already shipping has existing
// users with no recorded stamp. Without help, Detect cannot tell those
// users apart from a genuinely fresh install — see WithBaselineVersion for
// how to avoid re-running first-run onboarding for all of them.
package firstrun

import (
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/semver"
)

// Kind classifies the version transition detected between the previously
// recorded stamp and the app's current version.
type Kind string

const (
	// Fresh means no version was ever recorded and WithBaselineVersion was
	// not set — nothing distinguishes this from a first-ever launch.
	Fresh Kind = "fresh"
	// Upgrade means the current version is newer than the recorded one.
	Upgrade Kind = "upgrade"
	// Downgrade means the current version is older than the recorded one
	// (e.g. the user reinstalled an older build, or a rollback).
	Downgrade Kind = "downgrade"
	// Same means the current version matches the recorded one exactly.
	Same Kind = "same"
)

// Info describes one detected transition, passed to every Hook's When and
// Run functions.
type Info struct {
	Kind Kind
	// Previous is the zero semver.Version when Kind is Fresh. For all other
	// Kinds it is the recorded (or, under WithBaselineVersion, assumed)
	// prior version.
	Previous semver.Version
	// Current is always the version the Service was constructed with
	// (WithVersion).
	Current semver.Version
}

// Event name and payload emitted by Service.Run after a successful run
// (hooks succeeded and the stamp was updated).
const (
	// EventTransition fires once per successful Run call.
	EventTransition = "firstrun:transition"
)

// TransitionPayload is EventTransition's payload. Previous is "" for a
// Fresh transition (semver.Version's zero value has no meaningful string
// form); otherwise it is Info.Previous.String().
type TransitionPayload struct {
	Kind     Kind   `json:"kind"`
	Previous string `json:"previous"`
	Current  string `json:"current"`
}

// Error codes registered with the kit errors package.
const (
	// ErrConfig means New was called without enough configuration to
	// operate (missing WithVersion, or none of WithAppName/WithDirs/
	// WithStoragePath).
	ErrConfig errors.Code = "firstrun_config"
	// ErrStamp means the on-disk version stamp exists but could not be
	// parsed as a semver version — the file was likely hand-edited or
	// corrupted.
	ErrStamp errors.Code = "firstrun_stamp_invalid"
	// ErrHookFailed wraps whatever error a Hook's Run function returned.
	// The original error is preserved as Underlying (errors.Is/As and
	// errors.Unwrap see through it) — a Hook that returns ErrRefuse is
	// still detectable via errors.Is(err, firstrun.ErrRefuse) after Run
	// wraps it.
	ErrHookFailed errors.Code = "firstrun_hook_failed"
	// ErrDowngradeRefused is ErrRefuse's code — see ErrRefuse.
	ErrDowngradeRefused errors.Code = "firstrun_downgrade_refused"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrConfig:           i18n.T("wailskit.firstrun.errors.config", "First-run detection is misconfigured. Please contact support."),
		ErrStamp:            i18n.T("wailskit.firstrun.errors.stamp_invalid", "The recorded application version could not be read."),
		ErrHookFailed:       i18n.T("wailskit.firstrun.errors.hook_failed", "An update step failed to complete. Please try relaunching the app."),
		ErrDowngradeRefused: i18n.T("wailskit.firstrun.errors.downgrade_refused", "This version can't open data saved by a newer version of the app."),
	})
}

// ErrRefuse is a ready-made sentinel a Hook's Run function can return (or
// wrap) to signal "refuse to proceed" — the typical response to a
// downgrade a Hook decides is unsafe (data written by a newer version may
// be unreadable). Run does not special-case it: any error from a Hook stops
// remaining hooks and leaves the stamp untouched, exactly like any other
// hook failure. Its value is a stable, checkable identity: errors.Is(err,
// ErrRefuse) still works after Run wraps it in ErrHookFailed, since
// UserError.Unwrap returns the original error.
var ErrRefuse = errors.New(ErrDowngradeRefused, "firstrun: refused by hook", nil)
