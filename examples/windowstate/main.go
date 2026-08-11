// Command windowstate-example demonstrates the windowstate package: saving
// and restoring window geometry, clamped to a currently attached display.
//
// It compiles and vets headlessly on every platform (no build tag — AD-4).
// Actually *running* it opens a real window and therefore needs a live
// display/session (on Linux, gtk3 + webkit2gtk), like any Wails v3 app.
//
// Run it, move or resize the window, quit, and relaunch: the window comes
// back where you left it. What this example can't demonstrate is the
// package's actual reason for existing — unplug an external monitor a
// window was saved on and relaunch, and it still comes back on-screen
// instead of off it. That needs a real multi-monitor session; see
// windowstate/clamp_test.go for the automated coverage of that behavior
// against fabricated screens, and windowstate/README.md for how to verify
// it by hand.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jrschumacher/wails-kit/v2/state"
	"github.com/jrschumacher/wails-kit/v2/windowstate"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// A temp dir keeps this example self-contained and safe to run
	// repeatedly. A real app normally skips WithStore entirely and lets
	// Manage derive the path from the app's own name (Config().Name) plus
	// the WithName window key — see the README quickstart.
	tmp, err := os.MkdirTemp("", "wails-kit-windowstate-example-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	store, err := state.New[windowstate.Geometry](
		state.WithStoragePath[windowstate.Geometry](filepath.Join(tmp, "main.json")),
	)
	if err != nil {
		return err
	}

	app := application.New(application.Options{
		Name: "windowstate-example",
	})

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "main",
		Title:  "windowstate example",
		Width:  900,
		Height: 600,
		URL:    "data:text/html,<h1>windowstate example</h1><p>Move or resize this window, then quit. Relaunch to see it restored.</p>",
	})

	// Manage wires move/resize tracking immediately (debounced 300ms here
	// instead of the 500ms default, just to make the effect visible
	// quickly while trying this example out).
	mgr, err := windowstate.Manage(app, win,
		windowstate.WithName("main"),
		windowstate.WithStore(store),
		windowstate.WithDebounce(300*time.Millisecond),
	)
	if err != nil {
		return err
	}

	// Restore must run before the window is shown, so any previously saved
	// geometry (clamped to a currently attached display) is applied before
	// the first paint.
	mgr.Restore()

	fmt.Println("windowstate example running — move/resize the window, then quit and relaunch to see it restored.")
	fmt.Printf("geometry file: %s\n", store.Path())

	win.Show()
	err = app.Run()

	// Flush any pending debounced save on the way out so the last
	// drag/resize before quit isn't lost to the debounce window.
	mgr.Close()

	return err
}
