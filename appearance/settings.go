package appearance

import (
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// SettingsGroup's own catalog strings (locales/en.json) — settings.Field/
// Group.Label are i18n.Text (WP-12), resolved by whichever
// *i18n.Localizer the consuming settings.Service was built with
// (settings.WithLocalizer); this package does no resolution of its own.
var (
	groupLabel    = i18n.T("wailskit.appearance.group.label", "Appearance")
	modeLabel     = i18n.T("wailskit.appearance.mode.label", "Theme")
	modeDesc      = i18n.T("wailskit.appearance.mode.description", "Choose how the app looks, or follow your system setting.")
	modeOptSystem = i18n.T("wailskit.appearance.mode.option.system", "Follow system")
	modeOptLight  = i18n.T("wailskit.appearance.mode.option.light", "Light")
	modeOptDark   = i18n.T("wailskit.appearance.mode.option.dark", "Dark")
)

// SettingsGroup returns the settings.Group offering the theme picker
// (system/light/dark). Register it with your settings.Service via
// settings.WithGroup — that is a separate step from WithSettings, which
// only wires the *settings.Service reference this package reads/writes
// through; SettingsGroup builds the schema, it does not register it.
// SettingsGroupWithDefault is SettingsGroup with the mode field's default
// replaced.
//
// SettingsGroup defaults to ModeSystem, which is the right default for a new
// app. It is the wrong default for an app that previously shipped a single
// fixed theme: following the OS would change the appearance out from under
// every existing user on upgrade, without them asking. The first real adopter
// hit exactly this — it had shipped dark-only — and had to work around the
// hardcoded default by pre-seeding the settings file, which cost more code
// than the rest of the adoption combined.
func SettingsGroupWithDefault(mode Mode) settings.Group {
	g := SettingsGroup()
	for i := range g.Fields {
		if g.Fields[i].Key == SettingMode {
			g.Fields[i].Default = string(mode)
		}
	}
	return g
}

func SettingsGroup() settings.Group {
	return settings.Group{
		Key:   "appearance",
		Label: groupLabel,
		Fields: []settings.Field{
			{
				Key:         SettingMode,
				Type:        settings.FieldSelect,
				Label:       modeLabel,
				Description: modeDesc,
				Default:     string(ModeSystem),
				Options: []settings.SelectOption{
					{Value: string(ModeSystem), Label: modeOptSystem},
					{Value: string(ModeLight), Label: modeOptLight},
					{Value: string(ModeDark), Label: modeOptDark},
				},
			},
		},
	}
}
