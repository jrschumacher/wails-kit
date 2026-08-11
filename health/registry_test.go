package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	kiterrors "github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
)

// noopProbe never touches the network — used by every test that doesn't
// care about the built-in connectivity check's own behavior, so New's
// auto-registered default check never makes a real request.
type noopProbe struct{ err error }

func (p noopProbe) Probe(context.Context) error { return p.err }

func newTestRegistry(opts ...Option) *Registry {
	// Every test registry overrides the connectivity probe so New's
	// auto-registered default check never hits the real network — see
	// AGENTS.md "Landmines".
	all := append([]Option{WithConnectivityProbe(noopProbe{})}, opts...)
	return New(all...)
}

func TestNewRegistersDefaultConnectivityCheck(t *testing.T) {
	r := newTestRegistry()
	snap := r.Snapshot()
	if len(snap.Checks) != 1 || snap.Checks[0].Name != DefaultCheckName {
		t.Fatalf("expected default connectivity check registered, got %+v", snap.Checks)
	}
	if snap.Checks[0].State != StateUnknown {
		t.Fatalf("expected unprobed default check to be unknown, got %s", snap.Checks[0].State)
	}
}

func TestWithoutDefaultConnectivityCheck(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck())
	snap := r.Snapshot()
	if len(snap.Checks) != 0 {
		t.Fatalf("expected no checks registered, got %+v", snap.Checks)
	}
}

func TestUnregisterRemovesDefaultCheck(t *testing.T) {
	r := newTestRegistry()
	if !r.Unregister(DefaultCheckName) {
		t.Fatal("expected Unregister to report removal")
	}
	snap := r.Snapshot()
	if len(snap.Checks) != 0 {
		t.Fatalf("expected no checks after unregistering the default, got %+v", snap.Checks)
	}
	if r.Unregister(DefaultCheckName) {
		t.Fatal("expected second Unregister of the same name to report false")
	}
}

func TestDuplicateName(t *testing.T) {
	r := newTestRegistry()
	_, err := r.Register(Check{Name: DefaultCheckName, Probe: noopProbe{}})
	if err == nil {
		t.Fatal("expected duplicate registration to fail")
	}
	if !kiterrors.IsCode(err, ErrDuplicateCheck) {
		t.Fatalf("expected ErrDuplicateCheck, got %v", err)
	}
}

func TestRegisterRejectsInvalidCheck(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck())

	if _, err := r.Register(Check{Probe: noopProbe{}}); err == nil {
		t.Fatal("expected empty name to be rejected")
	}
	if _, err := r.Register(Check{Name: "x"}); err == nil {
		t.Fatal("expected nil probe to be rejected")
	}
}

func TestRegisterReturnsWorkingRemoveFunc(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck())
	remove, err := r.Register(Check{Name: "api", Probe: noopProbe{}})
	if err != nil {
		t.Fatal(err)
	}
	remove()
	remove() // must be safe to call twice
	if len(r.Snapshot().Checks) != 0 {
		t.Fatal("expected check removed after calling remove()")
	}
}

func TestRegistrationAndProbingAgainstHTTPTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := New(WithoutDefaultConnectivityCheck())
	if _, err := r.Register(Check{
		Name:  "api",
		Class: ClassBackend,
		Probe: HTTPProbe(srv.URL),
	}); err != nil {
		t.Fatal(err)
	}

	r.Trigger("api")

	snap := r.Snapshot()
	if len(snap.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(snap.Checks))
	}
	if snap.Checks[0].State != StateHealthy {
		t.Fatalf("expected healthy after probing a live httptest server, got %s (err=%q)", snap.Checks[0].State, snap.Checks[0].Err)
	}
	if snap.Checks[0].CheckedAt.IsZero() {
		t.Fatal("expected CheckedAt to be set after a probe")
	}
}

func TestProbeAgainstUnreachableAddress(t *testing.T) {
	// A closed listener on localhost: connection refused immediately,
	// deterministic, and never touches the real network — the documented
	// way to test failure without flakiness (see AGENTS.md Testing).
	ln := httptest.NewServer(nil)
	url := ln.URL
	ln.Close()

	r := New(WithoutDefaultConnectivityCheck())
	if _, err := r.Register(Check{Name: "api", Class: ClassBackend, Probe: HTTPProbe(url, WithTimeout(2*time.Second))}); err != nil {
		t.Fatal(err)
	}

	r.Trigger("api")

	snap := r.Snapshot()
	if snap.Checks[0].State != StateDown {
		t.Fatalf("expected down against an unreachable address, got %s", snap.Checks[0].State)
	}
	if snap.Checks[0].Err == "" {
		t.Fatal("expected a non-empty error message for a down check")
	}
}

func TestUnknownVersusHealthy(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck())
	if _, err := r.Register(Check{Name: "api", Class: ClassBackend, Critical: true, Probe: noopProbe{}}); err != nil {
		t.Fatal(err)
	}

	snap := r.Snapshot()
	if snap.Checks[0].State != StateUnknown {
		t.Fatalf("a never-probed check must report unknown, not healthy; got %s", snap.Checks[0].State)
	}
	if snap.Overall != StateUnknown {
		t.Fatalf("Overall must not claim healthy while a critical check is unprobed; got %s", snap.Overall)
	}

	r.Trigger("api")

	snap = r.Snapshot()
	if snap.Checks[0].State != StateHealthy {
		t.Fatalf("expected healthy after a successful probe, got %s", snap.Checks[0].State)
	}
	if snap.Overall != StateHealthy {
		t.Fatalf("expected overall healthy once the only critical check is healthy, got %s", snap.Overall)
	}
}

func TestEmptyRegistryOverallIsUnknown(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck())
	if got := r.Snapshot().Overall; got != StateUnknown {
		t.Fatalf("expected StateUnknown for an empty registry, got %s", got)
	}
}

func TestCriticalDownProducesDownOverall(t *testing.T) {
	r := newTestRegistry()
	if _, err := r.Register(Check{Name: "api", Class: ClassBackend, Critical: true, Probe: noopProbe{err: errors.New("boom")}}); err != nil {
		t.Fatal(err)
	}
	r.Trigger(DefaultCheckName, "api") // connectivity healthy (noopProbe{}), api down

	snap := r.Snapshot()
	if snap.Overall != StateDown {
		t.Fatalf("expected overall down when a critical check is down, got %s", snap.Overall)
	}
}

func TestNonCriticalDownDoesNotAffectOverall(t *testing.T) {
	r := newTestRegistry()
	if _, err := r.Register(Check{Name: "optional", Class: ClassProvider, Critical: false, Probe: noopProbe{err: errors.New("boom")}}); err != nil {
		t.Fatal(err)
	}
	r.Trigger(DefaultCheckName, "optional")

	snap := r.Snapshot()
	if snap.Overall != StateHealthy {
		t.Fatalf("a non-critical check going down must not affect Overall; got %s", snap.Overall)
	}
	var found bool
	for _, c := range snap.Checks {
		if c.Name == "optional" {
			found = true
			if c.State != StateDown {
				t.Fatalf("the non-critical check itself should still report down, got %s", c.State)
			}
		}
	}
	if !found {
		t.Fatal("expected the non-critical check in Checks")
	}
}

func TestOfflineSuppressesBlame(t *testing.T) {
	r := New(WithConnectivityProbe(noopProbe{err: errors.New("no network")}))
	if _, err := r.Register(Check{Name: "backend", Class: ClassBackend, Critical: true, Probe: noopProbe{err: errors.New("unreachable")}}); err != nil {
		t.Fatal(err)
	}
	r.Trigger(DefaultCheckName, "backend")

	snap := r.Snapshot()
	if !snap.Offline {
		t.Fatal("expected Offline=true when the connectivity check is down")
	}

	var backend CheckStatus
	for _, c := range snap.Checks {
		if c.Name == "backend" {
			backend = c
		}
	}
	if backend.State != StateUnknown {
		t.Fatalf("expected backend check suppressed to unknown while offline, got %s", backend.State)
	}
	// Overall still reflects reality: worst state among critical checks
	// (connectivity itself is critical and down) is StateDown, not
	// StateUnknown — "you're offline" is itself a real down state.
	if snap.Overall != StateDown {
		t.Fatalf("expected overall down while offline, got %s", snap.Overall)
	}
}

func TestOfflineDoesNotSuppressConnectivityItself(t *testing.T) {
	r := New(WithConnectivityProbe(noopProbe{err: errors.New("no network")}))
	r.Trigger(DefaultCheckName)

	snap := r.Snapshot()
	if snap.Checks[0].State != StateDown {
		t.Fatalf("the connectivity check's own state must remain down, not be suppressed to unknown; got %s", snap.Checks[0].State)
	}
}

func TestManualTrigger(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck(), WithDefaultInterval(time.Hour))
	if _, err := r.Register(Check{Name: "api", Class: ClassBackend, Probe: noopProbe{}}); err != nil {
		t.Fatal(err)
	}

	if got := r.Snapshot().Checks[0].State; got != StateUnknown {
		t.Fatalf("expected unknown before any probe, got %s", got)
	}

	// Trigger overrides the (very long) configured cadence and probes now.
	r.Trigger("api")

	if got := r.Snapshot().Checks[0].State; got != StateHealthy {
		t.Fatalf("expected Trigger to probe immediately regardless of cadence, got %s", got)
	}
}

func TestTriggerUnknownNameIsNoop(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck())
	r.Trigger("does-not-exist") // must not panic or block
}

func TestTransitionEventsOnly(t *testing.T) {
	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)
	r := New(WithEmitter(emitter), WithoutDefaultConnectivityCheck())

	if _, err := r.Register(Check{Name: "api", Class: ClassBackend, Probe: noopProbe{}}); err != nil {
		t.Fatal(err)
	}

	// Repeated healthy probes: only the first (unknown -> healthy)
	// transition should emit.
	r.Trigger("api")
	r.Trigger("api")
	r.Trigger("api")

	changed := mem.Events()
	if len(changed) != 1 {
		t.Fatalf("expected exactly 1 health:changed event for 3 identical-state probes, got %d: %+v", len(changed), changed)
	}
	payload, ok := changed[0].Data.(ChangedPayload)
	if !ok {
		t.Fatalf("expected ChangedPayload, got %T", changed[0].Data)
	}
	if payload.Check.Name != "api" || payload.Check.State != StateHealthy {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	mem.Clear()

	// Now flip to down: must emit again.
	r2Err := errors.New("down now")
	if err := r.Unregister("api"); !err {
		t.Fatal("expected unregister to succeed")
	}
	if _, err := r.Register(Check{Name: "api", Class: ClassBackend, Probe: noopProbe{err: r2Err}}); err != nil {
		t.Fatal(err)
	}
	r.Trigger("api")
	r.Trigger("api")

	changed = mem.Events()
	if len(changed) != 1 {
		t.Fatalf("expected exactly 1 event for the unknown->down transition plus repeats, got %d", len(changed))
	}
}
