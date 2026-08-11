package kit

import (
	"io/fs"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/appearance"
	"github.com/jrschumacher/wails-kit/v2/firstrun"
	"github.com/jrschumacher/wails-kit/v2/health"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"
	"github.com/jrschumacher/wails-kit/v2/updates"
)

// Option configures New. Every Option composes regardless of the order it's
// passed in — New collects the whole set into a config before constructing
// anything (same convention as settings.ServiceOption / appearance.Option),
// so e.g. WithoutHealth and WithHealthCheck may appear in either order and
// WithoutHealth always wins.
type Option func(*config)

type config struct {
	dirs                *appdirs.Dirs
	keyring             keyring.Store
	logLevel            string
	locale              string
	locales             []fs.FS
	settingsGroups      []settings.Group
	settingsStoragePath string

	withoutUpdates     bool
	withoutHealth      bool
	withoutDiagnostics bool

	githubOwner string
	githubRepo  string
	updatesOpts []updates.ServiceOption

	healthChecks []health.Check
	healthOpts   []health.Option

	firstRunHooks    []firstrun.Hook
	firstRunBaseline string

	appearanceSource    appearance.Source
	appearanceSourceSet bool

	appearanceDefaultMode    appearance.Mode
	appearanceDefaultModeSet bool
}

func newConfig() *config { return &config{} }

// WithDirs injects an already-constructed appdirs.Dirs instead of having
// New derive one from AppInfo.Name via appdirs.New. Primarily for tests
// (pair with appdirs.WithConfigDir(t.TempDir()) etc.) and for a consumer
// that already built a Dirs for other reasons. Every other kit-owned path
// (settings, logging, keyring, firstrun, diagnostics) is rooted under
// whichever Dirs ends up in use, so this one option redirects the whole
// tree — see AGENTS.md, "Wiring order".
func WithDirs(d *appdirs.Dirs) Option {
	return func(c *config) { c.dirs = d }
}

// WithKeyring overrides the default secrets backend — an EnvelopeStore
// wrapping the OS keyring, keyed by AppInfo.Name (see AD-8,
// docs/v2-roadmap.md) — with any keyring.Store. Pass keyring.NewMemoryStore()
// in tests; never construct a real keyring.NewOSStore in one (see
// keyring/AGENTS.md).
func WithKeyring(store keyring.Store) Option {
	return func(c *config) { c.keyring = store }
}

// WithLogLevel sets the logger's minimum level: "debug", "info" (the
// default), "warn", or "error". See logging.Config.Level.
func WithLogLevel(level string) Option {
	return func(c *config) { c.logLevel = level }
}

// WithLocale forces the initial resolved locale, taking precedence over
// every other i18n resolution tier (persisted settings override,
// environment, OS, configured default) — see i18n.WithLocale. Typical use:
// a CLI's --locale flag.
func WithLocale(tag string) Option {
	return func(c *config) { c.locale = tag }
}

// WithLocales adds an app catalog source (a filesystem of
// locales/<bcp47>.json files) merged over the kit's own and every other kit
// package's embedded catalog — see i18n.WithCatalog. Call more than once to
// add more than one source; later sources win on key collision.
func WithLocales(fsys fs.FS) Option {
	return func(c *config) { c.locales = append(c.locales, fsys) }
}

// WithSettingsGroup adds an app-defined settings.Group to the schema, after
// every kit-internal group that ends up enabled (locale, appearance,
// health, updates, diagnostics — see AGENTS.md, "Wiring order"). Call more
// than once to add more than one group; groups appear in the order given.
func WithSettingsGroup(g settings.Group) Option {
	return func(c *config) { c.settingsGroups = append(c.settingsGroups, g) }
}

// WithSettingsStoragePath overrides the default OS-standard settings file
// path with an explicit one — see settings.WithStoragePath. This is what an
// app with workspace-local, git-tracked config (docs/v2-roadmap.md §1,
// Prune) needs: settings living alongside project files instead of in the
// OS config directory. Password/secret fields still go through Keyring
// regardless of this option.
func WithSettingsStoragePath(path string) Option {
	return func(c *config) { c.settingsStoragePath = path }
}

// WithoutUpdates force-disables Updates regardless of WithGitHubRepo:
// Kit.Updates stays nil, no updates settings group is registered, and no
// updates lifecycle ticker runs. Updates are already off by default —
// WithGitHubRepo is what turns them on — so this exists for a caller that
// wants them unconditionally off even if repo info might otherwise appear
// elsewhere in its option list (e.g. assembled programmatically).
func WithoutUpdates() Option {
	return func(c *config) { c.withoutUpdates = true }
}

// WithoutHealth force-disables Kit.Health: no registry is constructed, no
// health settings group is registered, and nothing — including Updates, if
// enabled — can register a check against it.
func WithoutHealth() Option {
	return func(c *config) { c.withoutHealth = true }
}

// WithoutDiagnostics force-disables Kit.Diag: no service is constructed and
// no consent settings group is registered. diagnostics.RecoverAndLog and
// diagnostics.SettingsGroup remain usable as package-level functionality;
// only the kit-managed *diagnostics.Service instance is withheld.
func WithoutDiagnostics() Option {
	return func(c *config) { c.withoutDiagnostics = true }
}

// WithGitHubRepo enables Updates, checking owner/repo's GitHub releases.
// This is the only thing that turns updates on (see WithoutUpdates for the
// override, and WithUpdatesOptions for further configuration such as
// signature verification).
func WithGitHubRepo(owner, repo string) Option {
	return func(c *config) { c.githubOwner = owner; c.githubRepo = repo }
}

// WithUpdatesOptions passes additional updates.ServiceOption values through
// to updates.NewService — e.g. updates.WithPublicKey for signature
// verification (strongly recommended for any app that ships updates —
// see updates/AGENTS.md) or updates.WithAssetPattern. Only meaningful
// alongside WithGitHubRepo; a no-op if updates end up disabled.
func WithUpdatesOptions(opts ...updates.ServiceOption) Option {
	return func(c *config) { c.updatesOpts = append(c.updatesOpts, opts...) }
}

// WithHealthCheck registers an additional health.Check once Kit.Health is
// constructed. A no-op if health is disabled (WithoutHealth).
func WithHealthCheck(check health.Check) Option {
	return func(c *config) { c.healthChecks = append(c.healthChecks, check) }
}

// WithHealthOptions passes additional health.Option values through to
// health.New — most notably health.WithConnectivityProbe (every real app
// with its own notion of "online" should set this — see health/AGENTS.md)
// and health.WithoutDefaultConnectivityCheck (tests: the built-in default
// otherwise makes a real network request). A no-op if health is disabled
// (WithoutHealth).
func WithHealthOptions(opts ...health.Option) Option {
	return func(c *config) { c.healthOpts = append(c.healthOpts, opts...) }
}

// WithFirstRunHook appends a firstrun.Hook. Registration order is
// evaluation order across all WithFirstRunHook calls — see firstrun.Hook.
func WithFirstRunHook(h firstrun.Hook) Option {
	return func(c *config) { c.firstRunHooks = append(c.firstRunHooks, h) }
}

// WithFirstRunBaseline sets firstrun's WithBaselineVersion — required for
// an app adopting kit (and therefore firstrun) mid-life, so its existing
// installs are detected as Upgrade rather than Fresh on their next launch.
// See firstrun.WithBaselineVersion for the full rationale and limitation if
// omitted.
func WithFirstRunBaseline(version string) Option {
	return func(c *config) { c.firstRunBaseline = version }
}

// WithAppearanceSource pre-seeds the OS theme signal Kit.Appearance
// resolves ModeSystem against, equivalent to calling
// Kit.SetAppearanceSource immediately after New returns. kit/wailsbridge
// (WP-31) is the intended caller for a GUI app; a CLI/TUI process that wants
// real theme detection without ever attaching to Wails can supply its own
// appearance.Source here. Unset (the default): Kit.Appearance reports
// light until something calls SetAppearanceSource — see
// appearance_source.go's doc comment for why.
func WithAppearanceSource(s appearance.Source) Option {
	return func(c *config) { c.appearanceSource = s; c.appearanceSourceSet = true }
}

// WithAppearanceDefaultMode overrides the default value of the built-in
// appearance settings group's mode field.
//
// kit.New registers appearance.SettingsGroup(), which defaults to
// ModeSystem — correct for a new app, wrong for one that previously shipped
// a single fixed theme. Following the OS would change the appearance out
// from under every existing user on upgrade without them asking, so such an
// app wants ModeDark or ModeLight as its default and follow-OS as opt-in.
//
// This exists because the alternative was unreachable: WithSettingsGroup
// appends, so passing a second group with the same key produces two
// competing groups rather than an override. The first real adopter worked
// around it by pre-seeding the settings file before calling kit.New, which
// cost more code than the rest of its adoption combined.
func WithAppearanceDefaultMode(mode appearance.Mode) Option {
	return func(c *config) {
		c.appearanceDefaultMode = mode
		c.appearanceDefaultModeSet = true
	}
}
