// Command firstrun-example demonstrates the firstrun.Service: a fresh
// install, a multi-version upgrade that crosses several migration
// boundaries in one jump, and a downgrade that a hook refuses. It needs no
// network access or GUI — firstrun is a Wails-free package (not on the
// AD-4 allowlist) that runs identically in a CLI/TUI entry point.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/firstrun"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dir, err := os.MkdirTemp("", "firstrun-example")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	stampPath := filepath.Join(dir, "firstrun.json")

	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	// migrateWorkspaceLayout et al. record which migrations ran, standing
	// in for real data-migration work an app would do.
	var ranMigrations []string
	migration := func(name string) firstrun.Hook {
		return firstrun.Hook{
			Name: name,
			When: firstrun.OnUpgradeThrough(name),
			Run: func(_ context.Context, info firstrun.Info) error {
				ranMigrations = append(ranMigrations, name)
				fmt.Printf("  running migration for %s (upgrading %s -> %s)\n", name, info.Previous, info.Current)
				return nil
			},
		}
	}

	// --- Step 1: fresh install at 1.0.0 ---
	fmt.Println("=== fresh install at v1.0.0 ===")
	fresh, err := firstrun.New(
		firstrun.WithStoragePath(stampPath),
		firstrun.WithVersion("1.0.0"),
		firstrun.WithEmitter(emitter),
		firstrun.WithHooks(firstrun.Hook{
			Name: "welcome",
			When: firstrun.OnFresh(),
			Run: func(context.Context, firstrun.Info) error {
				fmt.Println("  welcome! this is your first launch.")
				return nil
			},
		}),
	)
	if err != nil {
		return fmt.Errorf("new fresh service: %w", err)
	}
	info, err := fresh.Run(context.Background())
	if err != nil {
		return fmt.Errorf("run fresh service: %w", err)
	}
	fmt.Printf("  result: kind=%s current=%s\n\n", info.Kind, info.Current)

	// --- Step 2: user skips releases entirely, jumps straight to 1.4.0 ---
	// Every intervening migration (1.1.0, 1.2.0, 1.3.0, 1.4.0) must run, in
	// that order — not just the 1.4.0 one.
	fmt.Println("=== upgrade straight from v1.0.0 to v1.4.0 ===")
	upgrade, err := firstrun.New(
		firstrun.WithStoragePath(stampPath),
		firstrun.WithVersion("1.4.0"),
		firstrun.WithEmitter(emitter),
		firstrun.WithHooks(
			migration("1.1.0"),
			migration("1.2.0"),
			migration("1.3.0"),
			migration("1.4.0"),
		),
	)
	if err != nil {
		return fmt.Errorf("new upgrade service: %w", err)
	}
	info, err = upgrade.Run(context.Background())
	if err != nil {
		return fmt.Errorf("run upgrade service: %w", err)
	}
	fmt.Printf("  result: kind=%s previous=%s current=%s\n", info.Kind, info.Previous, info.Current)
	fmt.Printf("  migrations run, in order: %v\n\n", ranMigrations)

	// --- Step 3: an older build launches against 1.4.0's data ---
	fmt.Println("=== downgrade to v1.2.0 (refused) ===")
	downgrade, err := firstrun.New(
		firstrun.WithStoragePath(stampPath),
		firstrun.WithVersion("1.2.0"),
		firstrun.WithEmitter(emitter),
		firstrun.WithHooks(firstrun.Hook{
			Name: "refuse-downgrade",
			When: firstrun.OnDowngrade(),
			Run: func(_ context.Context, info firstrun.Info) error {
				fmt.Printf("  refusing: data on disk was written by %s, this build is %s\n", info.Previous, info.Current)
				return firstrun.ErrRefuse
			},
		}),
	)
	if err != nil {
		return fmt.Errorf("new downgrade service: %w", err)
	}
	_, err = downgrade.Run(context.Background())
	switch {
	case err == nil:
		return errors.New("expected the downgrade to be refused")
	case errors.Is(err, firstrun.ErrRefuse):
		fmt.Println("  refusal confirmed via errors.Is(err, firstrun.ErrRefuse)")
	default:
		return fmt.Errorf("unexpected error: %w", err)
	}

	// The stamp is untouched by the refused downgrade — still 1.4.0.
	still, err := firstrun.New(firstrun.WithStoragePath(stampPath), firstrun.WithVersion("1.4.0"))
	if err != nil {
		return fmt.Errorf("new verification service: %w", err)
	}
	info, err = still.Detect()
	if err != nil {
		return fmt.Errorf("detect: %w", err)
	}
	fmt.Printf("  stamp after refusal: kind=%s (still recorded at v1.4.0)\n\n", info.Kind)

	fmt.Printf("%d firstrun:transition event(s) emitted (the refused downgrade did not emit one):\n", mem.Count())
	for _, e := range mem.Events() {
		payload := e.Data.(firstrun.TransitionPayload) //nolint:errcheck,forcetypeassert // example
		fmt.Printf("  - %s: %q -> %q\n", payload.Kind, payload.Previous, payload.Current)
	}

	return nil
}
