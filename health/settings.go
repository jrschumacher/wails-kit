package health

import (
	"fmt"
	"time"

	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// SettingCheckInterval is the settings key SettingsGroup's field is
// registered under. Its value is a time.Duration.String() (e.g. "30s",
// "1m0s"), parseable with time.ParseDuration. The registry does not read
// this setting itself — the app reads the submitted value and passes it to
// WithDefaultInterval (at next construction) or a specific Check's
// Interval, the same "app reads it, app decides" pattern
// updates.SettingCheckFrequency uses.
const SettingCheckInterval = "health.check_interval"

// intervalChoices are the fixed options SettingsGroup offers. The
// registry's own configured default interval (WithDefaultInterval) is
// always included even if it isn't one of these, so Default never points
// at a value missing from Options.
var intervalChoices = []time.Duration{
	15 * time.Second,
	30 * time.Second,
	time.Minute,
	2 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
}

// SettingsGroup's own catalog strings (health/locales/en.json). Field/Group
// Label (and SelectOption.Label) are i18n.Text — settings.Service resolves
// them against its own localizer in GetSchema (nil localizer -> literal
// Other values); health itself does no resolution.
var (
	groupLabel    = i18n.T("wailskit.health.settings.group_label", "Connectivity")
	fieldLabel    = i18n.T("wailskit.health.settings.check_interval.label", "Check interval")
	fieldDesc     = i18n.T("wailskit.health.settings.check_interval.description", "How often to probe registered endpoints.")
	labelEvery15s = i18n.T("wailskit.health.settings.interval.15s", "Every 15 seconds")
	labelEvery30s = i18n.T("wailskit.health.settings.interval.30s", "Every 30 seconds")
	labelEvery1m  = i18n.T("wailskit.health.settings.interval.1m", "Every minute")
	labelEvery2m  = i18n.T("wailskit.health.settings.interval.2m", "Every 2 minutes")
	labelEvery5m  = i18n.T("wailskit.health.settings.interval.5m", "Every 5 minutes")
	labelEvery15m = i18n.T("wailskit.health.settings.interval.15m", "Every 15 minutes")
)

// intervalLabels maps the fixed intervalChoices to their Text — a lookup
// table instead of a formula, since "every N" reads more naturally as fixed
// translated strings ("Every minute", not "Every 1 minutes") than anything
// a generic formula would produce without real plural-rule support.
var intervalLabels = map[time.Duration]i18n.Text{
	15 * time.Second: labelEvery15s,
	30 * time.Second: labelEvery30s,
	time.Minute:      labelEvery1m,
	2 * time.Minute:  labelEvery2m,
	5 * time.Minute:  labelEvery5m,
	15 * time.Minute: labelEvery15m,
}

// intervalOption builds one SelectOption for d. A d outside intervalLabels
// (only possible via a WithDefaultInterval value that isn't one of the six
// fixed choices) gets an untranslated literal Text — same convention as
// i18n.LocaleOptions' dynamic locale names (Key "" — never looked up in a
// catalog, see i18n/settings.go).
func intervalOption(d time.Duration) settings.SelectOption {
	if t, ok := intervalLabels[d]; ok {
		return settings.SelectOption{Value: d.String(), Label: t}
	}
	return settings.SelectOption{Value: d.String(), Label: i18n.Text{Other: fmt.Sprintf("Every %s", d)}}
}

// SettingsGroup returns a settings.Group with one field, a check-interval
// picker, for the app to surface in its settings UI. See
// SettingCheckInterval for how the submitted value is meant to be used.
func (r *Registry) SettingsGroup() settings.Group {
	r.mu.Lock()
	current := r.defaultInterval
	r.mu.Unlock()

	durations := make([]time.Duration, len(intervalChoices))
	copy(durations, intervalChoices)
	haveCurrent := false
	for _, d := range durations {
		if d == current {
			haveCurrent = true
			break
		}
	}
	if !haveCurrent {
		durations = append(durations, current)
	}

	options := make([]settings.SelectOption, 0, len(durations))
	for _, d := range durations {
		options = append(options, intervalOption(d))
	}

	return settings.Group{
		Key:   "health",
		Label: groupLabel,
		Fields: []settings.Field{
			{
				Key:         SettingCheckInterval,
				Type:        settings.FieldSelect,
				Label:       fieldLabel,
				Description: fieldDesc,
				Default:     current.String(),
				Options:     options,
			},
		},
	}
}
