package i18n

import (
	"sort"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/jrschumacher/wails-kit/v2/settings"
)

// SettingsGroup returns a settings.Group with one field, a locale picker,
// whose options are "system" (SettingsGroup's default — defers to the
// env/OS/default resolution tiers) plus every locale actually present in
// l's merged catalog. Field labels are plain strings here, matching
// settings.Field's current string-typed Label/Description/Placeholder;
// settings gains i18n.Text-typed fields in WP-12, and this group is not
// meant to pre-empt that.
func (l *Localizer) SettingsGroup() settings.Group {
	l.mu.RLock()
	keys := make([]string, 0, len(l.catalogs))
	for k := range l.catalogs {
		keys = append(keys, k)
	}
	l.mu.RUnlock()
	sort.Strings(keys)

	options := make([]settings.SelectOption, 0, len(keys)+1)
	options = append(options, settings.SelectOption{
		Value: "system",
		Label: l.T(systemLocaleLabel),
	})
	for _, k := range keys {
		options = append(options, settings.SelectOption{
			Value: k,
			Label: localeDisplayName(k),
		})
	}

	return settings.Group{
		Key:   "i18n",
		Label: "Language",
		Fields: []settings.Field{
			{
				Key:     SettingLocale,
				Type:    settings.FieldSelect,
				Label:   "Language",
				Default: "system",
				Options: options,
			},
		},
	}
}

// systemLocaleLabel is a kit-catalog string (see locales/en.json) rather
// than a literal, so an app catalog can override it like any other kit
// string.
var systemLocaleLabel = T("wailskit.i18n.locale_option.system", "System default")

// localeDisplayName renders tag as an English-language display name (e.g.
// "fr" -> "French"), falling back to the raw tag string if display has
// nothing for it (or tag doesn't parse).
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
