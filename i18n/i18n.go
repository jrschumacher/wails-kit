// Package i18n is the kit's single translation catalog. Go owns it — not
// just the frontend — because Prune's Cobra CLI and Bubbletea TUI need
// localized output too, and a frontend-only catalog cannot reach them
// (see docs/v2-roadmap.md AD-5).
//
// A Text is declared inline at the call/definition site, pairing a stable
// catalog key with the built-in (English, source-language) fallback:
//
//	var ErrNotFound = i18n.T("myapp.example.not_found", "not found")
//
// A Localizer resolves Text values against merged locale catalogs loaded
// from fs.FS sources (locales/<bcp47>.json), with CLDR plural support via
// golang.org/x/text. Kit packages embed their own locales/en.json; apps
// supply their own catalogs via WithCatalog, and app entries win over kit
// entries on key collision.
package i18n

// Text is a translatable string: a stable catalog key plus the built-in
// (English) fallback text used when no catalog entry resolves the key.
type Text struct {
	Key   string
	Other string
}

// T is sugar for constructing a Text literal at the call site, e.g.
//
//	var Greeting = i18n.T("myapp.example.greeting", "Hello, %s!")
func T(key, other string) Text {
	return Text{Key: key, Other: other}
}
