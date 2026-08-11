//go:build darwin

package appearance

import (
	"os/exec"
	"strings"
)

// readAppleInterfaceStyle is an unexported package-level func var — exactly
// the pattern i18n/os_locale_darwin.go uses for AppleLanguages — so no test
// in this package ever shells out; tests substitute it and restore the
// original afterward.
//
// `defaults read -g AppleInterfaceStyle` prints "Dark" when the OS is in
// dark mode. In light mode the key doesn't exist at all: the command exits
// non-zero with no output — there is no "Light" value to check for.
var readAppleInterfaceStyle = defaultReadAppleInterfaceStyle

func defaultReadAppleInterfaceStyle() (string, error) {
	out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
	return strings.TrimSpace(string(out)), err
}

// newDefaultSource returns the no-Wails default Source on macOS, used
// automatically by NewService when WithSource isn't given (mirrors i18n's
// automatic osLocales detection tier — see i18n/os_locale_darwin.go and
// docs/v2-roadmap.md's WP-13 entry).
func newDefaultSource() Source {
	return &darwinSource{dark: readInitialDark()}
}

// readInitialDark resolves IsDark once, at construction, and is never
// called again — see darwinSource's doc comment for why. A read failure
// (missing `defaults` binary, no GUI session, unexpected output) is
// treated as light, not an error: appearance always has *a* resolved
// theme, and light is the documented safe default (Source's doc comment).
func readInitialDark() bool {
	out, err := readAppleInterfaceStyle()
	return err == nil && out == "Dark"
}

// darwinSource is the no-Wails default Source: it resolves IsDark once, at
// construction, and caches it for the life of the Source. Without a Wails
// event loop there is no live signal to poll for OS theme flips while the
// process is running, so Subscribe is inert — see Source's doc comment. A
// live-updating source needs the Wails ThemeChanged event and is
// kit/wailsbridge's job (WP-31).
type darwinSource struct {
	dark bool
}

func (s *darwinSource) IsDark() bool { return s.dark }

func (s *darwinSource) Subscribe(func(dark bool)) (cancel func()) {
	return func() {}
}
