// Package kit is the wired-components constructor for a Wails v3 desktop
// app: appdirs, logging, events, keyring, i18n, settings, appearance,
// health, firstrun, updates, diagnostics, and lifecycle, composed in the
// order that makes each one work correctly, with one call to New instead of
// a dozen. It imports no Wails code — see AGENTS.md, "kit is Wails-free" —
// so a Cobra CLI or Bubbletea TUI entry point can call kit.New and kit.Start
// exactly like a GUI entry point does, with no application.App anywhere in
// the process.
//
// kit.New is deliberately not a composition root: it returns a *Kit of
// wired components, not a running application.App. The GUI entry point
// additionally calls kit/wailsbridge.Attach(k, app) to wire the same *Kit
// into a developer-owned Wails app. See docs/v2-roadmap.md, AD-1, for the
// full rationale — this package implements that decision; it does not
// revisit it.
package kit

import (
	"context"
	stderrors "errors"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/appearance"
	"github.com/jrschumacher/wails-kit/v2/diagnostics"
	kiterrors "github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/firstrun"
	"github.com/jrschumacher/wails-kit/v2/health"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/lifecycle"
	"github.com/jrschumacher/wails-kit/v2/logging"
	"github.com/jrschumacher/wails-kit/v2/settings"
	"github.com/jrschumacher/wails-kit/v2/updates"
)

// AppInfo identifies the app being bootstrapped. Name drives appdirs, the
// keyring service name (and its env-var fallback prefix), the settings
// file, and the log directory — pick it once and don't change it, or every
// one of those relocates. Version must parse as semver (see semver.ParseVersion);
// it drives firstrun's transition detection and, when updates are enabled,
// the current-version comparison.
type AppInfo struct {
	Name    string // e.g. "prune" — drives appdirs, keyring service name, log paths
	ID      string // bundle id, e.g. "com.aboldnewlook.prune" — not used by kit itself; carried for wailsbridge/template consumers that need it
	Version string // semver, injected at build time, e.g. "1.4.0" or "v1.4.0"
}

// Kit is the wired-components spine every entry point of an app builds on.
// Every field is populated by New in the order documented in AGENTS.md,
// "Wiring order"; a field is nil only when its owning component was
// disabled via a Without* Option (Updates, Diag) — every other field is
// always non-nil after a successful New.
type Kit struct {
	Info       AppInfo
	Dirs       *appdirs.Dirs
	Logger     *slog.Logger
	Events     *events.Emitter
	Keyring    keyring.Store
	Settings   *settings.Service
	I18n       *i18n.Localizer
	Appearance *appearance.Service
	Health     *health.Registry // nil when WithoutHealth()
	Updates    *updates.Service // nil unless WithGitHubRepo() (and not WithoutUpdates())
	FirstRun   *firstrun.Service
	Diag       *diagnostics.Service // nil when WithoutDiagnostics()
	Lifecycle  *lifecycle.Manager

	eventsBackend    *dynamicBackend
	appearanceSource *dynamicSource
}

// New wires up a Kit: appdirs, logging, events, keyring, i18n, settings,
// appearance, health, firstrun, updates, and diagnostics, in that order,
// then a lifecycle.Manager that knows how to start/stop the ones that run in
// the background. See AGENTS.md, "Wiring order" for why this exact order
// and not another.
//
// Every component is on by default (opt-out, not opt-in) except Updates,
// which is off unless WithGitHubRepo is given — there is nothing to check
// for updates against without a repo. A failure anywhere returns a non-nil
// error identifying which component failed; use GetComponent to inspect it
// programmatically.
func New(info AppInfo, opts ...Option) (*Kit, error) {
	if info.Name == "" {
		return nil, componentErr("kit", stderrors.New("AppInfo.Name is required"))
	}

	cfg := newConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	k := &Kit{Info: info}

	// 1. appdirs — everything else that touches disk (logging, settings,
	// keyring, firstrun, diagnostics) is rooted under these paths, so
	// nothing after this point may derive its own independent appdirs.Dirs
	// (see AGENTS.md landmine on updates' internal appdirs.New call, the one
	// exception kit cannot close from the outside).
	dirs := cfg.dirs
	if dirs == nil {
		dirs = appdirs.New(info.Name)
	}
	if err := dirs.EnsureAll(); err != nil {
		return nil, componentErr("appdirs", err)
	}
	k.Dirs = dirs

	// 2. logging — needs a log dir, hence after appdirs. logging.Init is
	// process-global state (see AGENTS.md landmine); safe here because New
	// is expected to run once near process startup.
	if err := logging.Init(&logging.Config{
		AppName:       info.Name,
		LogDir:        dirs.Log(),
		Level:         cfg.logLevel,
		SensitiveKeys: []string{"password", "token", "api_key", "secret"},
	}); err != nil {
		return nil, componentErr("logging", err)
	}
	k.Logger = logging.Get().Logger

	// 3. events — every later component takes this same Emitter. Its
	// backend starts as a drop-everything placeholder (dynamicBackend); a
	// CLI/TUI entry point never changes that, a GUI entry point retargets it
	// via SetEventsBackend (kit/wailsbridge.Attach, WP-31).
	backend := &dynamicBackend{}
	k.eventsBackend = backend
	k.Events = events.NewEmitter(backend)

	// 4. keyring — before settings, which needs it for secret fields.
	kr := cfg.keyring
	if kr == nil {
		envelope, err := keyring.NewEnvelopeStore(dirs, keyring.NewOSStore(info.Name, keyring.WithEnvPrefix(strings.ToUpper(info.Name))))
		if err != nil {
			return nil, componentErr("keyring", err)
		}
		kr = envelope
	}
	k.Keyring = kr

	// settingsPath is where the real settings.Service (step 6) will persist
	// — computed once so the i18n peek (step 5) reads the exact file the
	// real Service will later write, and so both stay rooted under k.Dirs
	// (not settings' own independent default, which would silently diverge
	// from a WithDirs override — see AGENTS.md landmine).
	settingsPath := cfg.settingsStoragePath
	if settingsPath == "" {
		settingsPath = filepath.Join(dirs.Config(), "settings.json")
	}

	// 5. i18n — before settings, because settings.LocaleGroup needs a
	// built Localizer to enumerate its locale picker options. i18n reads a
	// persisted locale override (if any) via a peek at the settings file
	// settings.Service will use — see settings_peek.go and AGENTS.md,
	// "Wiring order: the i18n/settings cycle" for why a Service can't be
	// used directly here.
	peek := settingsPeek{store: settings.NewStore(info.Name, settings.WithPath(settingsPath))}
	i18nOpts := []i18n.Option{
		i18n.WithSettings(peek),
		i18n.WithEmitter(k.Events),
	}
	if cfg.locale != "" {
		i18nOpts = append(i18nOpts, i18n.WithLocale(cfg.locale))
	}
	for _, fsys := range cfg.locales {
		i18nOpts = append(i18nOpts, i18n.WithCatalog(fsys))
	}
	localizer, err := i18n.New(i18nOpts...)
	if err != nil {
		return nil, componentErr("i18n", err)
	}
	k.I18n = localizer
	// Package-level: errors.GetUserMessage and UserError's JSON marshaling
	// resolve through this from now on, process-wide (see AGENTS.md
	// landmine — same caveat as logging.Init).
	kiterrors.SetLocalizer(localizer)

	// 6. settings — kit-internal groups are registered here, in the fixed
	// order documented in AGENTS.md, before any app-supplied group
	// (WithSettingsGroup). appearance's group is always present (appearance
	// itself is never optional); health/updates/diagnostics groups are
	// registered only if that component ends up enabled.
	settingsOpts := []settings.ServiceOption{
		settings.WithStoragePath(settingsPath),
		settings.WithKeyring(k.Keyring),
		settings.WithLocalizer(k.I18n),
		settings.WithGroup(settings.LocaleGroup(k.I18n)),
		settings.WithGroup(appearanceGroup(cfg)),
	}

	if !cfg.withoutHealth {
		healthOpts := append([]health.Option{health.WithEmitter(k.Events)}, cfg.healthOpts...)
		k.Health = health.New(healthOpts...)
		for _, c := range cfg.healthChecks {
			if _, err := k.Health.Register(c); err != nil {
				return nil, componentErr("health", err)
			}
		}
		settingsOpts = append(settingsOpts, settings.WithGroup(k.Health.SettingsGroup()))
	}

	updatesEnabled := !cfg.withoutUpdates && cfg.githubOwner != "" && cfg.githubRepo != ""
	if updatesEnabled {
		settingsOpts = append(settingsOpts, settings.WithGroup(updates.SettingsGroup()))
	}

	if !cfg.withoutDiagnostics {
		settingsOpts = append(settingsOpts, settings.WithGroup(diagnostics.SettingsGroup()))
	}

	for _, g := range cfg.settingsGroups {
		settingsOpts = append(settingsOpts, settings.WithGroup(g))
	}

	k.Settings = settings.NewService(settingsOpts...)

	// 7. appearance — always on; see appearance_source.go for why kit
	// always wraps with a dynamicSource rather than passing WithSource
	// directly (or omitting it and taking appearance's own platform
	// default).
	k.appearanceSource = &dynamicSource{}
	switch {
	case cfg.appearanceSourceSet:
		k.appearanceSource.setReal(cfg.appearanceSource)
	default:
		// Seed with the platform's no-Wails source (nil off darwin) so a
		// headless CLI or TUI on macOS still reflects the real OS theme.
		// Without this the indirection would report light everywhere until
		// wailsbridge called SetAppearanceSource — which never happens in a
		// process with no App, i.e. exactly the case kit exists to serve.
		if src := appearance.NewOSSource(); src != nil {
			k.appearanceSource.setReal(src)
		}
	}
	k.Appearance = appearance.NewService(
		appearance.WithSource(k.appearanceSource),
		appearance.WithSettings(k.Settings),
		appearance.WithEmitter(k.Events),
	)
	// Generic settings writes (settings.Binding.SetValues from a webview
	// form, settings/cli, ...) go through Settings.SetValues directly, not
	// appearance.Service.SetMode — AddOnChange is how appearance finds out
	// (see settings/service.go's doc comment on AddOnChange, added for
	// exactly this composition gap).
	appearanceSvc := k.Appearance
	k.Settings.AddOnChange(func(map[string]any) { appearanceSvc.Refresh() })

	// 8. firstrun — Detect (not Run) here: cheap, no side effects, surfaces
	// a corrupt version stamp as a construction error instead of deferring
	// it to Start. Hooks run in Start, once, before Lifecycle.Startup — see
	// Start.
	firstrunOpts := []firstrun.Option{
		firstrun.WithDirs(dirs),
		firstrun.WithVersion(normalizeVersion(info.Version)),
		firstrun.WithEmitter(k.Events),
	}
	if cfg.firstRunBaseline != "" {
		firstrunOpts = append(firstrunOpts, firstrun.WithBaselineVersion(cfg.firstRunBaseline))
	}
	if len(cfg.firstRunHooks) > 0 {
		firstrunOpts = append(firstrunOpts, firstrun.WithHooks(cfg.firstRunHooks...))
	}
	k.FirstRun, err = firstrun.New(firstrunOpts...)
	if err != nil {
		return nil, componentErr("firstrun", err)
	}
	if _, err := k.FirstRun.Detect(); err != nil {
		return nil, componentErr("firstrun", err)
	}

	// 9. updates — opt-in via WithGitHubRepo (see updatesEnabled above).
	if updatesEnabled {
		updatesOpts := []updates.ServiceOption{
			updates.WithGitHubRepo(cfg.githubOwner, cfg.githubRepo),
			updates.WithCurrentVersion(normalizeVersion(info.Version)),
			updates.WithEmitter(k.Events),
			updates.WithSettings(k.Settings),
			updates.WithAppName(info.Name),
		}
		if k.Health != nil {
			updatesOpts = append(updatesOpts, updates.WithHealth(k.Health))
		}
		updatesOpts = append(updatesOpts, cfg.updatesOpts...)
		k.Updates, err = updates.NewService(updatesOpts...)
		if err != nil {
			return nil, componentErr("updates", err)
		}
	}

	// 10. diagnostics — on by default (opt-out via WithoutDiagnostics).
	if !cfg.withoutDiagnostics {
		diagOpts := []diagnostics.ServiceOption{
			diagnostics.WithAppName(info.Name),
			diagnostics.WithVersion(normalizeVersion(info.Version)),
			diagnostics.WithDirs(dirs),
			diagnostics.WithSettings(k.Settings),
			diagnostics.WithEmitter(k.Events),
			diagnostics.WithFirstRun(k.FirstRun),
		}
		if k.Health != nil {
			diagOpts = append(diagOpts, diagnostics.WithHealth(k.Health))
		}
		k.Diag, err = diagnostics.NewService(diagOpts...)
		if err != nil {
			return nil, componentErr("diagnostics", err)
		}
	}

	// 11. lifecycle — registers the startable services (health's probe
	// loop, updates' check-frequency ticker), in dependency order: updates
	// depends on health only when both are enabled, so updates' first check
	// never races health's own registration of the github-releases check
	// (see updates.WithHealth, wired above).
	lifecycleOpts := []lifecycle.ManagerOption{lifecycle.WithEmitter(k.Events)}
	if k.Health != nil {
		lifecycleOpts = append(lifecycleOpts, lifecycle.WithService("health", &healthTicker{reg: k.Health}))
	}
	if k.Updates != nil {
		var updSvcOpts []lifecycle.ServiceOption
		if k.Health != nil {
			updSvcOpts = append(updSvcOpts, lifecycle.DependsOn("health"))
		}
		lifecycleOpts = append(lifecycleOpts, lifecycle.WithService("updates", &updatesTicker{svc: k.Updates, settings: k.Settings}, updSvcOpts...))
	}
	k.Lifecycle, err = lifecycle.NewManager(lifecycleOpts...)
	if err != nil {
		return nil, componentErr("lifecycle", err)
	}

	return k, nil
}

// Start runs firstrun's hooks (if this is the first launch, an upgrade, or
// a downgrade the hooks care about — see firstrun.Service.Run) and then
// starts every lifecycle-managed background service (health probing,
// updates ticking) in dependency order. Call it once, near the top of
// main, after New.
func (k *Kit) Start(ctx context.Context) error {
	if k.FirstRun != nil {
		if _, err := k.FirstRun.Run(ctx); err != nil {
			return componentErr("firstrun", err)
		}
	}
	if err := k.Lifecycle.Startup(ctx); err != nil {
		return componentErr("lifecycle", err)
	}
	return nil
}

// Close stops every lifecycle-managed background service (reverse startup
// order), then releases everything else that holds a resource: Appearance's
// OS-source subscription and the Events emitter's pending
// debounce/batch/async-handler state. Safe to call even if Start was never
// called or failed partway (lifecycle.Manager.Shutdown is a no-op unless
// Startup previously succeeded).
func (k *Kit) Close() error {
	var err error
	if shutErr := k.Lifecycle.Shutdown(); shutErr != nil {
		err = componentErr("lifecycle", shutErr)
	}
	k.Appearance.Close()
	k.Events.Close()
	return err
}

// SetEventsBackend retargets Kit.Events at a real Backend — the seam
// kit/wailsbridge.Attach (WP-31) uses to wire a Wails app.EmitEvent into an
// already-constructed Kit. CLI/TUI entry points never call this; every
// event emitted through Kit.Events (or anything built from it — Settings,
// Appearance, Health, ...) before this is called is silently dropped (see
// events.go).
func (k *Kit) SetEventsBackend(b events.Backend) {
	k.eventsBackend.Set(b)
}

// SetAppearanceSource retargets Kit.Appearance's OS theme signal — the seam
// kit/wailsbridge.Attach (WP-31) uses to wire a live Wails-backed
// appearance.Source (application.Env.IsDarkMode() +
// events.Common.ThemeChanged) into the already-constructed
// appearance.Service held at Kit.Appearance. Also available directly for a
// CLI/TUI consumer that wants a real OS signal without a Wails app — see
// appearance_source.go's doc comment for why kit.New doesn't wire one in on
// its own even on darwin.
//
// Calling this refreshes Kit.Appearance immediately, so Resolved() reflects
// the new source's current state without waiting for its next change
// notification.
func (k *Kit) SetAppearanceSource(s appearance.Source) {
	k.appearanceSource.setReal(s)
	k.Appearance.Refresh()
}

// appearanceGroup returns the built-in appearance settings group, honouring
// WithAppearanceDefaultMode if the caller set one.
func appearanceGroup(cfg *config) settings.Group {
	if cfg.appearanceDefaultModeSet {
		return appearance.SettingsGroupWithDefault(cfg.appearanceDefaultMode)
	}
	return appearance.SettingsGroup()
}

// normalizeVersion maps the conventional unset/dev version markers to a
// valid prerelease semver, and passes everything else through untouched.
//
// firstrun (and updates) require parseable semver, but the near-universal
// Go convention is a version variable defaulting to "dev" and overridden by
// ldflags at release time. Left alone, that hard-fails kit.New for every
// `go run`, `go build` without flags, and `go test` — i.e. the entire
// development loop, which is the worst possible place to fail. The first
// adopter hit this immediately.
//
// Only the two explicit markers are rewritten. Any other unparseable value
// is still a real misconfiguration and is left to fail loudly rather than
// silently becoming 0.0.0.
func normalizeVersion(v string) string {
	switch v {
	case "", "dev":
		return "0.0.0-dev"
	}
	return v
}
