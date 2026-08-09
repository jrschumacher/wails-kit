package settings

import "github.com/jrschumacher/wails-kit/v2/i18n"

// localeGroupLabel and localeFieldLabel are settings' own catalog strings
// (settings/locales/en.json) for the locale-picker group LocaleGroup builds.
var (
	localeGroupLabel = i18n.T("wailskit.settings.locale_group.label", "Language")
	localeFieldLabel = i18n.T("wailskit.settings.locale_group.field_label", "Language")
)

// LocaleGroup returns a settings.Group with one field — a locale picker —
// built from l's merged catalog via l.LocaleOptions(): "system" first
// (deferring to i18n's env/OS/default resolution tiers), then every locale
// actually present in the catalog.
//
// This lives here rather than as a method on *i18n.Localizer (its WP-10
// home, Localizer.SettingsGroup) because WP-12 gives settings.Field an
// i18n.Text-typed Label and settings.Service a *i18n.Localizer dependency
// (WithLocalizer) — so package settings now imports package i18n. The old
// Localizer.SettingsGroup needed settings.Group as its return type, and
// keeping both import directions would be a cycle (i18n -> settings ->
// i18n). See i18n/AGENTS.md's Landmines for the i18n-side half of this.
func LocaleGroup(l *i18n.Localizer) Group {
	opts := l.LocaleOptions()
	options := make([]SelectOption, len(opts))
	for i, o := range opts {
		options[i] = SelectOption{Value: o.Value, Label: o.Label}
	}

	return Group{
		Key:   "i18n",
		Label: localeGroupLabel,
		Fields: []Field{
			{
				Key:     i18n.SettingLocale,
				Type:    FieldSelect,
				Label:   localeFieldLabel,
				Default: "system",
				Options: options,
			},
		},
	}
}
