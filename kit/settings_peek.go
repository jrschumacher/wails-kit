package kit

import "github.com/jrschumacher/wails-kit/v2/settings"

// settingsPeek adapts a raw *settings.Store to i18n.SettingsSource so the
// Localizer can read a persisted locale override before the real
// *settings.Service exists. See AGENTS.md, "Wiring order: the i18n/settings
// cycle" for why a *settings.Service can't be used directly here: its
// knownKeys (and therefore what GetValues returns) are fixed by the groups
// registered at construction, and the i18n locale group
// (settings.LocaleGroup) can't be built without a Localizer that already
// exists — so something has to read the settings file before the schema
// that would normally gate it is defined.
//
// store.Load() has no such restriction: it returns whatever the file
// contains, unfiltered (Store.knownKeys is never set on this instance). A
// leftover unknown key is harmless — the *real* settings.Service constructed
// immediately afterward applies its own knownKeys as usual.
type settingsPeek struct {
	store *settings.Store
}

// GetValues implements i18n.SettingsSource.
func (p settingsPeek) GetValues() (map[string]any, error) {
	return p.store.Load()
}
