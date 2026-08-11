//go:build !darwin

package permissions

// otherPlatform is the platform implementation for every GOOS besides
// darwin. Per the roadmap (docs/v2-roadmap.md, WP-21): "Linux/Windows: all
// kinds return Unsupported in v2... Unsupported is acceptable." Windows
// does expose a notifications authorization concept via the Wails toast
// backend, but it degrades gracefully without a permission prompt at all
// (Windows Toast notifications don't require explicit user authorization
// the way macOS's UNUserNotificationCenter does), so there is nothing
// meaningful for Check/Request to report — StatusUnsupported is the
// honest answer, not a gap to fill later.
type otherPlatform struct{}

func newPlatform() platform {
	return otherPlatform{}
}

func (otherPlatform) check(Kind) (granted bool, supported bool) {
	return false, false
}

func (otherPlatform) request(Kind) (granted bool, supported bool, err error) {
	return false, false, nil
}

func (otherPlatform) openSystemSettings(Kind) error {
	return errUnsupportedPlatform
}
