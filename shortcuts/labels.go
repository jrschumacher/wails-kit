package shortcuts

import "github.com/jrschumacher/wails-kit/v2/i18n"

// Only app-specific menu items get an i18n.Text — standard roles
// (application.AddRole) render whatever label the OS itself supplies for
// About/Services/Hide/Quit/Edit/etc., and overriding that with our own
// catalog would produce a worse (and inconsistently-cased/punctuated)
// label than the platform's own localization. The Settings item is the one
// label this package authors itself, so it is the one label this package
// translates. See AGENTS.md, "Invariants".
var (
	// labelSettings is the non-macOS Settings item label, appended to the
	// Edit menu (see apply.go addEditMenuWithSettings).
	labelSettings = i18n.T("wailskit.shortcuts.settings.label", "Settings")
	// labelSettingsDarwin is the macOS Settings item label. It carries the
	// trailing ellipsis macOS Human Interface Guidelines use for menu items
	// that open a dialog/window rather than acting immediately — a
	// distinct catalog entry, not a suffix appended to labelSettings, since
	// a translator may need a different construction entirely (not every
	// language marks "opens a dialog" with a trailing "…").
	labelSettingsDarwin = i18n.T("wailskit.shortcuts.settings.label_darwin", "Settings…")
)

// resolveText resolves t through l, falling back to t.Other when l is nil
// — the same nil-is-opt-out contract settings/resolve.go uses. A Manager
// built without WithLocalizer (every Manager before WP-31, and every
// Manager today that doesn't opt in) resolves to the literal English
// strings above, so this is not a behavior change for existing callers.
func resolveText(t i18n.Text, l *i18n.Localizer) string {
	if l == nil {
		return t.Other
	}
	return l.T(t)
}
