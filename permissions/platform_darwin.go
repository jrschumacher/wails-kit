//go:build darwin

package permissions

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ApplicationServices -framework Foundation

#include <ApplicationServices/ApplicationServices.h>
#include <Foundation/Foundation.h>

// axIsProcessTrusted wraps AXIsProcessTrustedWithOptions. When prompt is
// true, macOS shows the "<App> would like to control this computer using
// accessibility features" dialog (if the app isn't already trusted) and
// adds it to the Accessibility list in System Settings — but this call
// still returns immediately with the *current* trust state; it does not
// block waiting for the user to flip the toggle. There is no callback or
// notification for "the user just granted it" short of polling
// AXIsProcessTrusted again.
static bool axIsProcessTrusted(bool prompt) {
	CFStringRef keys[1] = { kAXTrustedCheckOptionPrompt };
	CFTypeRef values[1] = { prompt ? kCFBooleanTrue : kCFBooleanFalse };
	CFDictionaryRef options = CFDictionaryCreate(
		kCFAllocatorDefault,
		(const void **)keys,
		(const void **)values,
		1,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks);
	bool trusted = AXIsProcessTrustedWithOptions(options);
	CFRelease(options);
	return trusted;
}

// hasValidMainBundle reports whether the running process has a real
// application bundle identifier. UNUserNotificationCenter (used by the
// Wails notifications service this package wraps) raises an *uncaught*
// NSInternalInconsistencyException — "bundleProxyForCurrentProcess is
// nil" — and hard-aborts the whole process (SIGABRT, not a catchable Go
// error) when called from a bare Mach-O binary with no Info.plist, which
// is exactly what `go run`/`go build` produce and what `go test` runs
// under. There is no Go-level recover for an uncaught Objective-C
// exception, so the only fix is to never call into
// UNUserNotificationCenter at all unless a real bundle identifier is
// present — checked here, once, before every Notifications call.
static bool hasValidMainBundle(void) {
	@autoreleasepool {
		NSBundle *bundle = [NSBundle mainBundle];
		return bundle != nil && bundle.bundleIdentifier != nil;
	}
}
*/
import "C"

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// darwinPlatform is the macOS platform implementation. See the package doc
// for why Check/Request collapse the OS's richer state into a
// granted/not-granted bool here and Service does the denied/not_determined
// synthesis.
type darwinPlatform struct{}

func newPlatform() platform {
	return darwinPlatform{}
}

func (darwinPlatform) check(k Kind) (granted bool, supported bool) {
	switch k {
	case Notifications:
		if !bool(C.hasValidMainBundle()) {
			// See hasValidMainBundle's doc: calling into
			// UNUserNotificationCenter here would hard-crash the process,
			// not return an error. Report Unsupported instead of risking
			// that — this is the common case for `go run`/`go test`/an
			// unsigned dev build.
			return false, false
		}
		ok, err := notifications.New().CheckNotificationAuthorization()
		if err != nil {
			// The Wails SDK call itself failed rather than reporting "not
			// authorized" — treat as unsupported rather than silently
			// claiming NotDetermined.
			return false, false
		}
		return ok, true
	case Accessibility:
		return bool(C.axIsProcessTrusted(false)), true
	case FullDiskAccess:
		return fullDiskAccessGranted(), true
	default:
		return false, false
	}
}

func (darwinPlatform) request(k Kind) (granted bool, supported bool, err error) {
	switch k {
	case Notifications:
		if !bool(C.hasValidMainBundle()) {
			// Same guard as check — never call into
			// UNUserNotificationCenter without a real bundle identifier.
			return false, false, nil
		}
		ok, reqErr := notifications.New().RequestNotificationAuthorization()
		if reqErr != nil {
			return false, true, reqErr
		}
		return ok, true, nil
	case Accessibility:
		// There is no programmatic grant. Passing prompt=true asks macOS
		// to show its own "trust this app?" dialog (if not already
		// trusted) and list the app in System Settings > Privacy &
		// Security > Accessibility; the call still returns immediately
		// with the pre-decision trust state, which is why Service.Request
		// racing this against ctx never actually blocks long for this
		// Kind in practice — the "may never return" risk here is about
		// the user, not this call.
		return bool(C.axIsProcessTrusted(true)), true, nil
	case FullDiskAccess:
		// No request API exists at all (per the roadmap). Request is
		// exactly OpenSystemSettings plus a fresh check.
		if openErr := openSettingsPane(paneFullDiskAccess); openErr != nil {
			return false, true, openErr
		}
		return fullDiskAccessGranted(), true, nil
	default:
		return false, false, nil
	}
}

func (darwinPlatform) openSystemSettings(k Kind) error {
	switch k {
	case Notifications:
		return openSettingsPane(paneNotifications)
	case Accessibility:
		return openSettingsPane(paneAccessibility)
	case FullDiskAccess:
		return openSettingsPane(paneFullDiskAccess)
	default:
		return errUnsupportedPlatform
	}
}

// System Settings deep-link URLs. These are the long-standing
// x-apple.systempreferences URLs Apple has kept working across System
// Preferences (pre-Ventura) and System Settings (Ventura+) for backward
// compatibility; they are not guaranteed by Apple to survive future macOS
// releases and cannot be exercised by an automated test (opening System
// Settings needs a live GUI session) — verify manually per AGENTS.md after
// any macOS upgrade.
const (
	paneNotifications  = "x-apple.systempreferences:com.apple.preference.notifications"
	paneAccessibility  = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
	paneFullDiskAccess = "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"
)

func openSettingsPane(url string) error {
	return exec.Command("open", url).Run()
}

// fullDiskAccessCanary is the path this package probes to infer Full Disk
// Access. There is no API for this permission at all (per the roadmap);
// ~/Library/Safari is one of the folders macOS's TCC subsystem protects
// unconditionally, regardless of the probing app's identity, and is
// present on every macOS install (Safari ships with the OS) — chosen over
// alternatives like ~/Library/Application Support/com.apple.TCC/TCC.db
// because it doesn't assume SIP/TCC internals that could change, only that
// Safari's support folder exists and is protected, which has been true
// since FDA was introduced in Mojave.
const fullDiskAccessCanary = "Library/Safari"

// fullDiskAccessGranted reports whether reading the canary path succeeds.
// A permission-denied error (EPERM, surfaced by Go as fs.ErrPermission) is
// the OS's actual denial signal; any other error (e.g. the path is
// missing) is treated as not-granted but is not a *reliable* denial signal
// either way — Service's seen-based synthesis is what turns "not granted"
// into Denied vs NotDetermined, so no error is lost, only a boolean is
// derived from it.
func fullDiskAccessGranted() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.ReadDir(filepath.Join(home, fullDiskAccessCanary))
	if err == nil {
		return true
	}
	// Both a permission-denied error (fs.ErrPermission — the real OS
	// denial signal) and any other error (e.g. the path is missing) fall
	// through to false here; Service's seen-based synthesis is what turns
	// "not granted" into Denied vs NotDetermined, so nothing is lost by
	// not branching on the specific error.
	return false
}
