// Command starter is a minimal Wails v3 application that wires together the
// three pieces a new wails-kit project needs:
//
//   - settings.NewService — schema-driven settings with keyring-backed secrets
//   - llm.LLMSettingsGroup — provider/model selection contributed to that schema
//   - app.Updater.Init — the framework's own auto-updater
//
// It is deliberately plain. The frontend is one hand-written HTML file with no
// build step, because the thing being demonstrated is the wiring, not the UI.
//
// See ../../docs/auto-update.md for the update half in full.
package main

import (
	"embed"
	"fmt"
	"log"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"

	"github.com/jrschumacher/wails-kit/llm"
	"github.com/jrschumacher/wails-kit/settings"

	// Provider implementations register themselves from init. Without these
	// blank imports the settings UI still lists Anthropic and OpenAI, but
	// llm.NewProviderFromValues cannot build either one.
	_ "github.com/jrschumacher/wails-kit/llm/anthropic"
	_ "github.com/jrschumacher/wails-kit/llm/openai"
)

//go:embed all:assets
var assets embed.FS

// updatePublicKey is the ed25519 trust anchor for update verification. It is
// the ONLY thing that authenticates a downloaded artifact — the release feed
// cannot substitute its own key.
//
// The checked-in key is a placeholder whose private half was destroyed, so this
// example fails closed: any signed release it is pointed at will be rejected.
// That is the correct default. Generate your own before shipping; see
// ../../docs/auto-update.md, "Key generation".
//
//go:embed update-public-key.pem
var updatePublicKey []byte

// version is the running version, and it MUST equal both
// CFBundleShortVersionString and CFBundleVersion in build/darwin/Info.plist
// (and `info.version` in build/config.yml, which generates them).
//
// A mismatch is silent and has two failure modes, neither of which logs
// anything: too low and the app offers the same update forever, too high and it
// never sees a real one. See docs/auto-update.md, "Version mismatch".
const version = "0.1.0"

// updateRepository is the GitHub repo releases are published to. Baked into
// every shipped binary — see docs/auto-update.md on why the feed location can
// never move.
const updateRepository = "jrschumacher/wails-kit-starter"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	prefs := NewUpdatePrefs(version)

	// The provider manager needs the service, and the service's onChange hook
	// needs the manager, so the variable is declared first and captured by the
	// closure. The hook cannot fire before NewService returns.
	var providers *llm.ProviderManager

	svc, err := settings.NewService(
		settings.WithAppName("wails-kit-starter"),
		settings.WithGroup(appSettingsGroup()),
		settings.WithGroup(llm.LLMSettingsGroup()),
		settings.WithGroup(prefs.Group()),
		settings.WithOnChange(func(map[string]any) {
			if providers == nil {
				return
			}
			// A reload failure here is not fatal: the manager drops its cached
			// provider, so the next Provider() call reports the same error to
			// whoever actually needs a model.
			if err := providers.Reload(); err != nil {
				log.Printf("llm: rebuilding provider after settings change: %v", err)
			}
		}),
	)
	if err != nil {
		// NewService fails when the config directory cannot be resolved or the
		// schema is malformed. Both are fatal and both are worth reporting at
		// startup rather than at first use.
		return fmt.Errorf("settings: %w", err)
	}

	// Corruption is not an error — the service is fully usable — but it means
	// the user's saved settings were unreadable and have been reset. Never
	// swallow it: a real app shows this in the UI, not just the log.
	if c := svc.Corruption(); c != nil {
		log.Printf("settings: config file %s was unreadable and has been reset to defaults; "+
			"the previous file was preserved at %s (%s)", c.Path, c.QuarantinePath, c.Reason)
	}

	providers = llm.NewProviderManager(svc)

	appSvc := &AppService{settings: svc, providers: providers, prefs: prefs}

	app := application.New(application.Options{
		Name:        "wails-kit starter",
		Description: "Settings, LLM provider management and auto-update, wired end to end.",
		Services: []application.Service{
			// Bind svc.Bindings(), NEVER svc itself.
			//
			// Wails binds every exported method of a bound type. The Service
			// exposes GetSecret and GetValuesWithSecrets, which return real API
			// keys; binding it would publish both to the webview and undo the
			// entire reason secrets live in the keyring. Bindings exposes
			// exactly GetSchema, GetValues, SetValues and Corruption.
			application.NewService(svc.Bindings()),
			application.NewService(appSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// app.Updater exists as soon as application.New returns; Init configures it.
	// application.New also calls updater.HandleHelperMode() internally, which is
	// what makes Restart work with no wiring on our side.
	appSvc.app = app

	if err := initUpdater(app, svc, prefs); err != nil {
		// A misconfigured updater is not a reason to refuse to start — the app
		// still works, it just cannot update itself. Report it loudly.
		log.Printf("updater: disabled: %v", err)
	}

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:           "wails-kit starter",
		Width:           720,
		Height:          720,
		URL:             "/",
		DevToolsEnabled: true,
	})

	return app.Run()
}

// initUpdater reads the update preferences out of the settings service and
// configures the framework updater from them.
//
// Config is read once, at startup, because updater.Init may only be called once
// per process: a second call returns updater.ErrAlreadyConfigured. Changing the
// channel or the auto-check toggle therefore takes effect on next launch, which
// is worth saying in the settings UI.
func initUpdater(app *application.App, svc *settings.Service, prefs *UpdatePrefs) error {
	values, err := svc.GetValues()
	if err != nil {
		return fmt.Errorf("read update settings: %w", err)
	}

	provider, err := github.New(github.Config{
		Repository: updateRepository,
		// Names a sibling asset on the release that the provider fetches and
		// parses to populate Release.Verification with a SHA-256 digest.
		// A digest proves integrity, not authenticity — the signature checked
		// against updatePublicKey is what proves the bytes came from you.
		ChecksumAsset: "checksums.txt",
		// Prerelease widens the search to include GitHub prereleases. Wire it
		// to the channel setting so a "beta" user sees them.
		Prerelease: channelOf(values) == ChannelBeta,
	})
	if err != nil {
		return err
	}

	cfg := updater.Config{
		CurrentVersion: version,
		Providers:      []updater.Provider{provider},
		PublicKey:      updatePublicKey,
		Channel:        channelOf(values),
	}

	// CheckInterval starts a background poller. Leave it zero and nothing
	// polls; the user drives updates from the Check Now button.
	if autoCheckEnabled(values) {
		cfg.CheckInterval = 6 * time.Hour
	}

	if err := app.Updater.Init(cfg); err != nil {
		return err
	}

	// Record the timestamp for the computed update.lastChecked field. The
	// updater emits this event for both manual and background checks.
	app.Event.On(updater.EventCheckStarted, func(*application.CustomEvent) {
		prefs.MarkChecked(time.Now())
	})

	return nil
}

// appSettingsGroup is an ordinary application settings group, here to show that
// the LLM and update groups are nothing special — they are the same Group value
// any application code can build.
func appSettingsGroup() settings.Group {
	return settings.Group{
		Key:   "app",
		Label: "General",
		Fields: []settings.Field{
			{
				Key:         "app.displayName",
				Type:        settings.FieldText,
				Label:       "Display name",
				Description: "Shown in the window title bar.",
				Default:     "wails-kit starter",
				Validation:  &settings.Validation{Required: true, MaxLen: 64},
			},
			{
				Key:     "app.telemetry",
				Type:    settings.FieldToggle,
				Label:   "Send anonymous usage data",
				Default: false,
			},
		},
	}
}
