package updates

import (
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// Settings keys.
const (
	SettingCheckFrequency     = "updates.check_frequency"
	SettingAutoDownload       = "updates.auto_download"
	SettingIncludePrereleases = "updates.include_prereleases"
)

// SettingsGroup's own catalog strings (updates/locales/en.json) — WP-12
// threads i18n through settings.Field/Group, so every label here is now
// i18n.Text rather than a plain string.
var (
	groupLabel           = i18n.T("wailskit.updates.settings.group_label", "Updates")
	checkFrequencyLabel  = i18n.T("wailskit.updates.settings.check_frequency.label", "Check for updates")
	checkFrequencyOnOpen = i18n.T("wailskit.updates.settings.check_frequency.option.startup", "On startup")
	checkFrequencyDaily  = i18n.T("wailskit.updates.settings.check_frequency.option.daily", "Daily")
	checkFrequencyWeekly = i18n.T("wailskit.updates.settings.check_frequency.option.weekly", "Weekly")
	checkFrequencyNever  = i18n.T("wailskit.updates.settings.check_frequency.option.never", "Never")
	autoDownloadLabel    = i18n.T("wailskit.updates.settings.auto_download.label", "Automatically download updates")
	prereleasesLabel     = i18n.T("wailskit.updates.settings.include_prereleases.label", "Include pre-release versions")
)

// SettingsGroup returns a settings.Group for update preferences.
func SettingsGroup() settings.Group {
	return settings.Group{
		Key:   "updates",
		Label: groupLabel,
		Fields: []settings.Field{
			{
				Key:     SettingCheckFrequency,
				Type:    settings.FieldSelect,
				Label:   checkFrequencyLabel,
				Default: "daily",
				Options: []settings.SelectOption{
					{Label: checkFrequencyOnOpen, Value: "startup"},
					{Label: checkFrequencyDaily, Value: "daily"},
					{Label: checkFrequencyWeekly, Value: "weekly"},
					{Label: checkFrequencyNever, Value: "never"},
				},
			},
			{
				Key:     SettingAutoDownload,
				Type:    settings.FieldToggle,
				Label:   autoDownloadLabel,
				Default: false,
			},
			{
				Key:      SettingIncludePrereleases,
				Type:     settings.FieldToggle,
				Label:    prereleasesLabel,
				Default:  false,
				Advanced: true,
			},
		},
	}
}
