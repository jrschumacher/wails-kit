// Command state-example demonstrates the state package: generic typed
// persistence with atomic writes, and an event emitter whose handlers can
// safely call back into the store (the WP-06 deadlock fix — Save/Load used
// to emit their change event while still holding the store's lock).
//
// It uses a temp directory via WithStoragePath so this example never writes
// into a real OS data directory — safe to run repeatedly and safe to run in
// CI.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/state"
)

// WindowState is a typical use: window geometry that doesn't need a schema,
// validation, or keyring integration — settings is for that.
type WindowState struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	X      int `json:"x"`
	Y      int `json:"y"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "wails-kit-state-example-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	// New enforces that at least one of WithAppName / WithStoragePath is
	// set — without a path there is nowhere to persist state, and prior to
	// WP-06 this went unenforced: Load would silently return defaults and
	// Save would fail later with a confusing os.Rename error.
	store, err := state.New[WindowState](
		state.WithStoragePath[WindowState](filepath.Join(tmp, "window.json")),
		state.WithDefaults[WindowState](WindowState{Width: 800, Height: 600}),
		state.WithEmitter[WindowState](emitter),
	)
	if err != nil {
		return err
	}

	// A handler that calls back into the store it's reacting to is a
	// natural thing to write (e.g. "on save, reload and log the effective
	// state"). Save/Load emit state:saved/state:loaded only after
	// releasing their internal lock, so this does not deadlock — it would
	// have, before WP-06.
	events.On(emitter, state.StateSaved, func(payload state.StateSavedPayload) {
		current, err := store.Load()
		if err != nil {
			log.Fatalf("reentrant load failed: %v", err)
		}
		fmt.Printf("state:saved handler observed %+v (store name: %q)\n", current, payload.Name)
	})

	// No file exists yet, so Load returns the configured defaults.
	initial, err := store.Load()
	if err != nil {
		return err
	}
	fmt.Printf("initial (defaults): %+v\n", initial)

	// Save writes atomically (temp file + fsync + rename) and then emits
	// state:saved, triggering the handler registered above.
	if err := store.Save(WindowState{Width: 1440, Height: 900, X: 100, Y: 50}); err != nil {
		return err
	}

	saved, err := store.Load()
	if err != nil {
		return err
	}
	fmt.Printf("after save: %+v\n", saved)

	if err := store.Delete(); err != nil {
		return err
	}
	afterDelete, err := store.Load()
	if err != nil {
		return err
	}
	fmt.Printf("after delete (back to defaults): %+v\n", afterDelete)

	return nil
}
