package i18n

import (
	"sort"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// LocaleOption is one entry in a locale picker: a value paired with its
// display label.
//
// WP-12 note: this package used to expose a Localizer.SettingsGroup()
// method that built a settings.Group directly (importing package settings
// for its Group/Field/SelectOption types). WP-12 makes settings.Field carry
// i18n.Text and settings.Service accept a *Localizer via WithLocalizer,
// which means package settings now imports package i18n. Keeping this
// package's former settings import too would create a hard cycle
// (i18n -> settings -> i18n), which Go refuses to build. LocaleOptions is
// the cycle-free replacement: it returns plain data, and settings.LocaleGroup
// (settings/locale.go) assembles it into a settings.Group. See
// settings/AGENTS.md "Dependencies & insulation" for the settings-side half
// of this note.
type LocaleOption struct {
	Value string
	Label Text
}

// LocaleOptions returns the locale-picker options for l's merged catalog:
// "system" first (the option a picker should default to — defers to the
// env/OS/default resolution tiers), then every locale actually present in
// the catalog, sorted, each labeled with an English-language display name.
func (l *Localizer) LocaleOptions() []LocaleOption {
	l.mu.RLock()
	keys := make([]string, 0, len(l.catalogs))
	for k := range l.catalogs {
		keys = append(keys, k)
	}
	l.mu.RUnlock()
	sort.Strings(keys)

	options := make([]LocaleOption, 0, len(keys)+1)
	options = append(options, LocaleOption{Value: "system", Label: systemLocaleLabel})
	for _, k := range keys {
		options = append(options, LocaleOption{Value: k, Label: T("", localeDisplayName(k))})
	}
	return options
}

// systemLocaleLabel is a kit-catalog string (see locales/en.json) rather
// than a literal, so an app catalog can override it like any other kit
// string.
var systemLocaleLabel = T("wailskit.i18n.locale_option.system", "System default")

// localeDisplayName renders tag as an English-language display name (e.g.
// "fr" -> "French"), falling back to the raw tag string if display has
// nothing for it (or tag doesn't parse). Deliberately not a catalog lookup
// (no per-locale-name translation) — an unadorned Text{Other: name} is
// exactly right for LocaleOption.Label.
func localeDisplayName(tag string) string {
	parsed, err := language.Parse(tag)
	if err != nil {
		return tag
	}
	if name := display.English.Languages().Name(parsed); name != "" {
		return name
	}
	return tag
}
