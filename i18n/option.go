package i18n

import (
	"io/fs"

	"github.com/jrschumacher/wails-kit/v2/events"
)

// Option configures a Localizer. See New.
type Option func(*Localizer)

// WithCatalog adds a catalog source: an fs.FS containing locales/<bcp47>.json
// files (values are a plain string or a {"one":..,"other":..} plural
// object). Sources merge in the order given to New, per key per locale, and
// the kit's own embedded catalog is always merged first — so later
// WithCatalog calls (i.e. the app's own) win over earlier ones (i.e. the
// kit's) on key collision.
func WithCatalog(fsys fs.FS) Option {
	return func(l *Localizer) {
		l.pendingCatalogs = append(l.pendingCatalogs, fsys)
	}
}

// WithLocale explicitly sets the locale, taking precedence over every other
// resolution tier (settings override, environment, OS source, default). tag
// is a BCP-47 language tag, e.g. "fr" or "pt-BR".
func WithLocale(tag string) Option {
	return func(l *Localizer) {
		l.explicit = tag
	}
}

// WithDefault sets the configured-default tier of the resolution order —
// the locale used when no higher tier resolves. Defaults to English ("en")
// if not given. An invalid tag is ignored (falls back to English) rather
// than failing New.
func WithDefault(tag string) Option {
	return func(l *Localizer) {
		l.defaultTagRaw = tag
	}
}

// SettingsSource is the minimal surface i18n needs from a settings service
// to read a persisted locale override, without importing package settings
// (which would be a needless coupling for a leaf package). *settings.Service
// satisfies this via its existing GetValues method.
type SettingsSource interface {
	GetValues() (map[string]any, error)
}

// SettingLocale is the settings key SettingsGroup's field is registered
// under, and the key WithSettings reads for the settings-override
// resolution tier. The special value "system" (SettingsGroup's default)
// means "no override — keep resolving lower tiers."
const SettingLocale = "i18n.locale"

// WithSettings wires a settings service as the settings-override resolution
// tier (OQ-5, tier 2): if the service has a non-empty, non-"system" value
// for SettingLocale, it wins over environment/OS/default. Read once at
// construction, like every other tier — see New.
func WithSettings(src SettingsSource) Option {
	return func(l *Localizer) {
		l.settingsSrc = src
	}
}

// WithEmitter wires an events.Emitter so SetLocale emits i18n:changed. Nil
// (the default — New without this option) means SetLocale updates state
// silently.
func WithEmitter(e *events.Emitter) Option {
	return func(l *Localizer) {
		l.emitter = e
	}
}
