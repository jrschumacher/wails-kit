package updates

import "github.com/jrschumacher/wails-kit/settings"

// Settings keys.
const (
	SettingCheckFrequency    = "updates.check_frequency"
	SettingAutoDownload      = "updates.auto_download"
	SettingIncludePrereleases = "updates.include_prereleases"
)

// SettingsGroup returns a settings.Group for update preferences.
func SettingsGroup() settings.Group {
	return settings.Group{
		Key:   "updates",
		Label: "Updates",
		Fields: []settings.Field{
			{
				Key:     SettingCheckFrequency,
				Type:    settings.FieldSelect,
				Label:   "Check for updates",
				Default: "daily",
				Options: []settings.SelectOption{
					{Label: "On startup", Value: "startup"},
					{Label: "Daily", Value: "daily"},
					{Label: "Weekly", Value: "weekly"},
					{Label: "Never", Value: "never"},
				},
			},
			{
				Key:     SettingAutoDownload,
				Type:    settings.FieldToggle,
				Label:   "Automatically download updates",
				Default: false,
			},
			{
				Key:     SettingIncludePrereleases,
				Type:    settings.FieldToggle,
				Label:   "Include pre-release versions",
				Default: false,
				Advanced: true,
			},
		},
	}
}
