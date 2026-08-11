package diagnostics

import (
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// SettingConsent is the settings key for the diagnostics submission consent
// toggle. It defaults to off: submitting a bundle is never a default-path
// behavior (see Submit).
const SettingConsent = "diagnostics.submission_consent"

// SettingsGroup returns the settings.Group offering the diagnostics
// submission consent toggle. Wire it into your app's settings.Service (see
// docs/settings-integration.md) and pass the same *settings.Service to
// WithSettings so Submit and SubmissionConsent can read it.
//
// The field defaults to false. There is deliberately no "on by default"
// option: a user must explicitly opt in before any bundle leaves the
// machine.
func SettingsGroup() settings.Group {
	return settings.Group{
		Key:   "diagnostics",
		Label: i18n.T("wailskit.diagnostics.group", "Diagnostics"),
		Fields: []settings.Field{
			{
				Key:         SettingConsent,
				Type:        settings.FieldToggle,
				Label:       i18n.T("wailskit.diagnostics.consent.label", "Share diagnostic bundles with support"),
				Description: i18n.T("wailskit.diagnostics.consent.description", "When enabled, you can submit diagnostics bundles (logs and system info) to support. Nothing is sent automatically or without this being on."),
				Default:     false,
			},
		},
	}
}

// SubmissionConsent reports whether the user has opted in to diagnostics
// bundle submission (SettingConsent in the wired settings.Service). It
// returns false — never — if no settings service is wired at all, if the
// value can't be read, or if the stored value isn't a bool; absence of
// proof of consent is treated as absence of consent.
//
// This is exposed (not just used internally by Submit) so composing code —
// notably the planned crash-sentinel — can check consent before deciding
// whether to offer/attempt an automatic submission, instead of
// reimplementing the same settings read.
func (s *Service) SubmissionConsent() bool {
	if s.settings == nil {
		return false
	}
	values, err := s.settings.GetValues()
	if err != nil {
		return false
	}
	consent, _ := values[SettingConsent].(bool)
	return consent
}
