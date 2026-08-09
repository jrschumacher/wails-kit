// Command lifecycle-example demonstrates lifecycle.Manager: dependency-
// ordered startup, health checks, and a clean shutdown, using in-memory
// stand-in services so this example needs no real database, network, or
// filesystem state — safe to run repeatedly and safe to run in CI.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/lifecycle"
)

// exampleService is a minimal lifecycle.Service. Real services wrap actual
// resources (a *sql.DB, an HTTP client, a background worker) instead of
// just printing.
type exampleService struct {
	name string
}

func (s *exampleService) OnStartup(_ context.Context) error {
	fmt.Printf("  starting %s\n", s.name)
	return nil
}

func (s *exampleService) OnShutdown() error {
	fmt.Printf("  stopping %s\n", s.name)
	return nil
}

// Health reports StatusHealthy unconditionally — implementing HealthChecker
// is optional; services that don't implement it default to healthy too.
func (s *exampleService) Health() lifecycle.HealthStatus {
	return lifecycle.StatusHealthy
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	db := &exampleService{name: "database"}
	settings := &exampleService{name: "settings"}
	updates := &exampleService{name: "updates"}

	mgr, err := lifecycle.NewManager(
		// updates depends on settings, settings depends on db — Startup
		// resolves this to db -> settings -> updates regardless of
		// registration order, and Shutdown reverses it.
		lifecycle.WithService("updates", updates, lifecycle.DependsOn("settings")),
		lifecycle.WithService("settings", settings, lifecycle.DependsOn("db")),
		lifecycle.WithService("db", db),
		lifecycle.WithEmitter(emitter),
		lifecycle.WithTimeout(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("new manager: %w", err)
	}

	fmt.Println("resolved startup order:", mgr.Order())

	fmt.Println("starting services...")
	if err := mgr.Startup(context.Background()); err != nil {
		return fmt.Errorf("startup: %w", err)
	}

	for _, h := range mgr.Health() {
		fmt.Printf("health: %s = %s\n", h.Name, h.Status)
	}

	fmt.Println("shutting down...")
	if err := mgr.Shutdown(); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	// A Manager is a single startup/shutdown cycle, not a free-standing
	// concurrent object: Shutdown left it idle, so Startup can run again
	// from a clean state, but calling Startup a second time *without* an
	// intervening Shutdown would return lifecycle_invalid_state instead of
	// double-starting everything. See README "Concurrency and reuse
	// contract".
	fmt.Println("starting again after a full shutdown cycle...")
	if err := mgr.Startup(context.Background()); err != nil {
		return fmt.Errorf("second startup: %w", err)
	}
	if err := mgr.Shutdown(); err != nil {
		return fmt.Errorf("second shutdown: %w", err)
	}

	fmt.Println("events emitted:", mem.Count())
	return nil
}
