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
// WireLocale wires svc's onChange hook so a locale picked through
// LocaleGroup's field (i18n.SettingLocale) takes effect immediately — not
// just on next launch (M6). Without this, SetValues persists the new value
// (the field is a perfectly ordinary select), the *next process start*
// picks it up via i18n.WithSettings' tier-2 resolution, but nothing tells
// the already-running Localizer to re-resolve, so the picker looks like it
// worked and silently doesn't until relaunch.
//
// Call once after constructing both:
//
//	l, _ := i18n.New(i18n.WithCatalog(appCatalog), i18n.WithSettings(svc))
//	svc := settings.NewService(settings.WithGroup(settings.LocaleGroup(l)), ...)
//	settings.WireLocale(svc, l)
//
// The special "system" value (LocaleGroup's default, meaning "defer to the
// env/OS/default resolution tiers") is deliberately not forwarded to
// l.SetLocale — "system" doesn't parse as a BCP-47 tag, and more
// fundamentally Localizer has no operation to "un-set" an explicit
// SetLocale back to those lower tiers once one has been made (see
// i18n/AGENTS.md's Landmines: resolution runs once, at New, and is cached).
// Concretely: picking French then later picking "System default" leaves the
// running process on French until relaunch — WireLocale prevents the
// spurious SetLocale("system") parse error, it does not add the "revert to
// system" capability i18n itself doesn't have. That asymmetry is
// intentional and documented here rather than silently half-working.
//
// Uses Service.AddOnChange, so this may be called at any point after both
// svc and l exist — it does not need to run before NewService.
func WireLocale(svc *Service, l *i18n.Localizer) {
	svc.AddOnChange(func(values map[string]any) {
		raw, ok := values[i18n.SettingLocale].(string)
		if !ok || raw == "" || raw == "system" {
			return
		}
		_ = l.SetLocale(raw)
	})
}

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
