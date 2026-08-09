// Command appearance-example demonstrates the appearance package headlessly:
// resolving system/light/dark, persisting a user override via settings,
// reacting to a simulated OS theme flip, and the Refresh() wiring for
// settings-UI changes that bypass SetMode.
//
// It uses a throwaway temp directory for the settings file, so this
// example never touches a real config directory — safe to run repeatedly
// and safe to run in CI. It never imports wails/v3 (AD-4) and never
// shells out — the OS source used here is a small in-process fake, not
// the real darwin `defaults read` source, so the example's behavior is
// identical on every platform and every machine.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/jrschumacher/wails-kit/v2/appearance"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "wails-kit-appearance-example-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// A real app wires appearance.WithEmitter to the same *events.Emitter
	// everything else uses (e.g. a Wails-backed one from wailsbridge). Here
	// we print every appearance:changed event as it fires.
	emitter := events.NewEmitter(events.BackendFunc(func(name string, data any) {
		fmt.Printf("event: %s %+v\n", name, data)
	}))

	// appearanceSvc is declared before settingsSvc and assigned after, so
	// settings.WithOnChange's closure can call back into it once it
	// exists — see the README's "Reacting to settings-UI changes" section
	// and appearance.Service.Refresh's doc comment for why this ordering
	// is needed.
	var appearanceSvc *appearance.Service
	settingsSvc := settings.NewService(
		settings.WithStoragePath(filepath.Join(tmp, "settings.json")),
		settings.WithGroup(appearance.SettingsGroup()),
		settings.WithOnChange(func(map[string]any) {
			if appearanceSvc != nil {
				appearanceSvc.Refresh()
			}
		}),
	)

	// demoSource is a stand-in for a real OS signal (the darwin default,
	// or a live Wails-backed one from kit/wailsbridge). It starts light.
	source := newDemoSource(false)

	appearanceSvc = appearance.NewService(
		appearance.WithSource(source),
		appearance.WithSettings(settingsSvc),
		appearance.WithEmitter(emitter),
	)
	defer appearanceSvc.Close()

	fmt.Println("mode:", appearanceSvc.Mode())         // "system" — the default
	fmt.Println("resolved:", appearanceSvc.Resolved()) // "light" — source starts light

	fmt.Println("\n--- OS flips to dark while on follow-system ---")
	source.flip(true)
	fmt.Println("resolved:", appearanceSvc.Resolved())

	fmt.Println("\n--- user pins Light explicitly (overrides the OS) ---")
	if err := appearanceSvc.SetMode(appearance.ModeLight); err != nil {
		return fmt.Errorf("set mode: %w", err)
	}
	fmt.Println("resolved:", appearanceSvc.Resolved())
	fmt.Println("further OS flips are now ignored:")
	source.flip(false)
	fmt.Println("resolved:", appearanceSvc.Resolved())

	fmt.Println("\n--- a generic settings form writes appearance.mode directly ---")
	fmt.Println("(bypassing SetMode entirely — this is what settings.Binding.SetValues")
	fmt.Println("looks like from the frontend's side of a rendered settings form)")
	if _, err := settingsSvc.SetValues(map[string]any{appearance.SettingMode: string(appearance.ModeDark)}); err != nil {
		return fmt.Errorf("set values: %w", err)
	}
	fmt.Println("mode (read live):", appearanceSvc.Mode())
	fmt.Println("resolved (settings.WithOnChange -> Refresh already ran):", appearanceSvc.Resolved())

	return nil
}

// demoSource is a minimal appearance.Source: a boolean plus a subscriber
// list, standing in for a real OS signal. Never use this in an app —
// appearance.WithSource takes the darwin default (automatic — see
// package README) or a live Wails-backed Source from kit/wailsbridge.
type demoSource struct {
	mu       sync.Mutex
	dark     bool
	handlers []func(bool)
}

func newDemoSource(dark bool) *demoSource {
	return &demoSource{dark: dark}
}

func (d *demoSource) IsDark() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dark
}

func (d *demoSource) Subscribe(fn func(dark bool)) (cancel func()) {
	d.mu.Lock()
	d.handlers = append(d.handlers, fn)
	idx := len(d.handlers) - 1
	d.mu.Unlock()
	return func() {
		d.mu.Lock()
		d.handlers[idx] = nil
		d.mu.Unlock()
	}
}

func (d *demoSource) flip(dark bool) {
	d.mu.Lock()
	d.dark = dark
	handlers := make([]func(bool), len(d.handlers))
	copy(handlers, d.handlers)
	d.mu.Unlock()
	for _, h := range handlers {
		if h != nil {
			h(dark)
		}
	}
}
