// Command kit-gui-example is a full miniature Wails v3 app built entirely
// from wails-kit: kit.New for the wired spine, kit/wailsbridge.Attach to
// wire it into a real *application.App (events, appearance, menu,
// frontend-safe bindings), and kit/wailsbridge.ManageWindow for window
// geometry persistence and background-colour sync. The frontend (embedded
// below) is a hand-written settings page — no framework, no generated
// bindings — calling the settings/i18n bindings directly through the
// Wails v3 runtime's window.wails.Call.ByName, to keep this example
// buildable and readable without a build step.
//
// It uses a throwaway temp directory and an in-memory keyring, the same as
// every other kit example, so running it never touches a real config
// directory or OS keychain.
//
// Run it with a display: `go run ./examples/kit-gui`. It also builds
// (but cannot usefully run) headlessly — `go build ./examples/kit-gui`
// succeeds in CI with no display attached.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/kit"
	"github.com/jrschumacher/wails-kit/v2/kit/wailsbridge"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// assets embeds the whole frontend/ directory. The "all:" prefix is
// required: without it, //go:embed silently excludes files and
// directories whose name starts with "." or "_" — a real risk for a real
// frontend build's output directory (Vite's default output includes
// hashed asset filenames, but tooling-generated dotfiles are common) —
// and, worse, produces no build error. The assets would simply be missing
// at runtime, in production, the one place nobody is watching for it.
//
//go:embed all:frontend
var assets embed.FS

// exampleGroup demonstrates an app registering its own settings group
// alongside the kit's built-in ones (locale, appearance, health,
// diagnostics) — see kit.WithSettingsGroup. GetSchema resolves this
// group's labels the same way it resolves the kit's own, so the frontend
// needs no special-casing for "which groups are ours".
var (
	exampleGroupLabel = i18n.T("example.kit_gui.group.label", "Example")
	exampleNameLabel  = i18n.T("example.kit_gui.name.label", "Your name")
	exampleNameDesc   = i18n.T("example.kit_gui.name.description", "Shown nowhere yet — this is just a demo field.")
)

var exampleGroup = settings.Group{
	Key:   "example",
	Label: exampleGroupLabel,
	Fields: []settings.Field{
		{Key: "example.name", Type: settings.FieldText, Label: exampleNameLabel, Description: exampleNameDesc},
	},
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "wails-kit-kit-gui-example-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	dirs := appdirs.New("kit-gui-example",
		appdirs.WithConfigDir(tmp+"/config"),
		appdirs.WithDataDir(tmp+"/data"),
		appdirs.WithCacheDir(tmp+"/cache"),
		appdirs.WithLogDir(tmp+"/log"),
		appdirs.WithTempDir(tmp+"/temp"),
	)

	k, err := kit.New(
		kit.AppInfo{Name: "kit-gui-example", ID: "dev.wails-kit.examples.kit-gui", Version: "1.0.0"},
		kit.WithDirs(dirs),
		kit.WithKeyring(keyring.NewMemoryStore()), // never a real OS keychain in an example
		kit.WithSettingsGroup(exampleGroup),
	)
	if err != nil {
		if comp, ok := kit.GetComponent(err); ok {
			return fmt.Errorf("kit.New: component %q failed: %w", comp, err)
		}
		return fmt.Errorf("kit.New: %w", err)
	}

	frontendFS, err := fs.Sub(assets, "frontend")
	if err != nil {
		return fmt.Errorf("fs.Sub(frontend): %w", err)
	}

	app := application.New(application.Options{
		Name: "kit-gui-example",
		Assets: application.AssetOptions{
			// BundledAssetFileServer (not the plain AssetFileServerFS)
			// additionally serves the compiled Wails runtime at
			// /wails/runtime.js, which frontend/index.html loads to get
			// window.wails.Call/Events — see frontend/app.js.
			Handler: application.BundledAssetFileServer(frontendFS),
		},
	})

	if err := wailsbridge.Attach(k, app); err != nil {
		return fmt.Errorf("wailsbridge.Attach: %w", err)
	}

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "kit-gui example",
		Width:  900,
		Height: 640,
		// Matches wailsbridge's own light default (appearance/README.md's
		// recommended value) — ManageWindow corrects this immediately to
		// whatever appearance.Resolved() actually is, and keeps it synced
		// from then on; this is just what's visible for the first frame
		// before that runs.
		BackgroundColour: application.NewRGB(255, 255, 255),
	})

	if err := wailsbridge.ManageWindow(k, app, win); err != nil {
		return fmt.Errorf("wailsbridge.ManageWindow: %w", err)
	}

	if err := k.Start(context.Background()); err != nil {
		return fmt.Errorf("k.Start: %w", err)
	}
	defer func() {
		if err := k.Close(); err != nil {
			log.Printf("k.Close: %v", err)
		}
	}()

	win.Show()
	return app.Run()
}
