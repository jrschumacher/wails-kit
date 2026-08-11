//go:build darwin

package i18n

import (
	"os/exec"
	"strings"
)

// osLocales is the OS-source tier of the resolution order (OQ-5, decided):
// `defaults read -g AppleLanguages`, not cgo NSLocale and not a
// wailsbridge-supplied value. cgo in this leaf package would complicate
// cross-compilation for every consumer — including Prune's CLI and TUI,
// which import this package directly and are not GUI processes — to fix a
// GUI-only detection problem. AppleLanguages returns the user's *ordered*
// language preference list, which is exactly what language.Matcher wants;
// AppleLocale alone would discard the fallback chain.
//
// This is an unexported package-level func var specifically so tests can
// substitute it — no test in this package ever shells out.
var osLocales = readAppleLanguages

// readAppleLanguages execs `defaults read -g AppleLanguages` and parses its
// plist-ish array output into an ordered list of BCP-47-ish tags, most
// preferred first. Any failure (missing binary, no GUI session, parse
// trouble) returns nil — per OQ-5, failure to read the OS source is not an
// error, it just falls through to the next resolution tier.
func readAppleLanguages() []string {
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return nil
	}
	return parseAppleLanguages(string(out))
}

// parseAppleLanguages parses the textual plist array `defaults read` prints,
// e.g.:
//
//	(
//	    "en-US",
//	    "fr-FR",
//	    ja
//	)
//
// into ["en-US", "fr-FR", "ja"]. Split out from readAppleLanguages so it can
// be unit tested against fixed input without shelling out.
func parseAppleLanguages(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")

	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"`)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
