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
