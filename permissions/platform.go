package permissions

// platform is the narrow slice of OS calls Service needs. It exists so
// Service's state machine — Status transitions, the seen-tracking that
// synthesizes denied vs not_determined (see the package doc), context
// handling in Request, and event emission — is testable against a fake
// instead of real cgo/Wails calls that need a live display/session to do
// anything meaningful. newPlatform (platform_darwin.go / platform_other.go)
// supplies the real, build-tagged implementation.
type platform interface {
	// check reports the current authorization state for k without
	// prompting. It returns (StatusGranted, true), (statusNotGranted,
	// true), or (StatusUnsupported, false) — the bool tells Service
	// whether the platform has an opinion at all; when it does not,
	// Service reports StatusUnsupported regardless of the Status value.
	// check never itself resolves the granted/not-granted split into
	// StatusDenied vs StatusNotDetermined — that synthesis (based on
	// whether this Kind has been "seen" this process) lives in Service so
	// it applies uniformly across all three Kinds and is exercised by one
	// set of tests instead of three platform-specific ones.
	check(k Kind) (granted bool, supported bool)

	// request triggers the OS-level authorization flow for k and blocks
	// until it resolves. It never receives a context: neither the
	// notifications SDK call nor AXIsProcessTrustedWithOptions accept
	// one. Service.Request layers context cancellation on top by racing
	// this call in a goroutine (see service.go) — a caller whose context
	// is cancelled gets control back immediately, at the cost of a
	// goroutine that keeps running until the underlying platform call
	// itself returns (documented in AGENTS.md as a landmine, not silently
	// hidden).
	//
	// For Accessibility and FullDiskAccess there is no programmatic
	// grant; request opens System Settings (or returns immediately after
	// triggering the OS prompt option) and reports whatever check would
	// report — see platform_darwin.go for the per-Kind contract.
	request(k Kind) (granted bool, supported bool, err error)

	// openSystemSettings opens the OS settings pane relevant to k.
	openSystemSettings(k Kind) error
}
