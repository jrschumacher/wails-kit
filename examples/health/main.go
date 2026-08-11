// Command health-example demonstrates the health.Registry: registering a
// critical backend endpoint and a non-critical provider endpoint against
// local httptest servers, triggering a manual probe, running the
// background scheduler briefly, and reading the resulting snapshot and
// events. It needs no real network access — the registry's own built-in
// connectivity check is overridden with a fake Probe for exactly that
// reason (see health/AGENTS.md, "Landmines": the default connectivity
// check otherwise makes a real request).
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/health"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// A healthy backend and a provider that's down — stand-ins for "your
	// own API" and "a third-party dependency", the two failure modes
	// health.Class distinguishes from "no network at all".
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	provider := httptest.NewServer(nil)
	providerURL := provider.URL
	provider.Close() // closed immediately: simulates an unreachable provider

	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	registry := health.New(
		health.WithEmitter(emitter),
		health.WithDefaultInterval(200*time.Millisecond),
		// Fake connectivity: this example is "online" by definition, so it
		// never touches the real network to prove it. A real app leaves
		// this unset (or points it at its own definition of "online") and
		// gets a real captive-portal check for free.
		health.WithConnectivityProbe(health.ProbeFunc(func(context.Context) error { return nil })),
	)

	remove, err := registry.Register(health.Check{
		Name:     "api",
		Class:    health.ClassBackend,
		Critical: true, // the app degrades to "offline mode" if this is down
		Probe:    health.HTTPProbe(backend.URL),
	})
	if err != nil {
		return fmt.Errorf("register backend check: %w", err)
	}
	defer remove()

	if _, err := registry.Register(health.Check{
		Name:     "weather-provider",
		Class:    health.ClassProvider,
		Critical: false, // a widget goes blank; the app itself is fine
		Probe:    health.HTTPProbe(providerURL, health.WithTimeout(500*time.Millisecond)),
	}); err != nil {
		return fmt.Errorf("register provider check: %w", err)
	}

	fmt.Println("before any probe:")
	printSnapshot(registry.Snapshot())

	fmt.Println("\ntriggering an immediate check of both endpoints...")
	registry.Trigger("api", "weather-provider")
	printSnapshot(registry.Snapshot())

	fmt.Println("\nrunning the background scheduler for ~500ms (interval 200ms)...")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	registry.Start(ctx) // blocks until ctx is Done, then returns — clean shutdown

	fmt.Println("\nfinal snapshot:")
	printSnapshot(registry.Snapshot())

	fmt.Printf("\n%d health:changed event(s) emitted (state transitions only — not one per probe):\n", mem.Count())
	for _, e := range mem.Events() {
		payload := e.Data.(health.ChangedPayload) //nolint:errcheck,forcetypeassert // example
		fmt.Printf("  - %s -> %s (overall now %s)\n", payload.Check.Name, payload.Check.State, payload.Overall)
	}

	return nil
}

func printSnapshot(s health.Snapshot) {
	fmt.Printf("  overall=%s offline=%v\n", s.Overall, s.Offline)
	for _, c := range s.Checks {
		fmt.Printf("  - %-16s class=%-12s critical=%-5v state=%s\n", c.Name, c.Class, c.Critical, c.State)
	}
}
