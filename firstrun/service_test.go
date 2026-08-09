package firstrun

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/events"
)

func newTestService(t *testing.T, opts ...Option) *Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), "firstrun.json")
	allOpts := append([]Option{WithStoragePath(path)}, opts...)
	svc, err := New(allOpts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

func TestNewRequiresVersion(t *testing.T) {
	_, err := New(WithStoragePath(filepath.Join(t.TempDir(), "firstrun.json")))
	if err == nil {
		t.Fatal("expected error when WithVersion is not provided")
	}
}

func TestNewRequiresStorageLocation(t *testing.T) {
	_, err := New(WithVersion("1.0.0"))
	if err == nil {
		t.Fatal("expected error when no storage location option is provided")
	}
}

func TestNewRejectsInvalidVersion(t *testing.T) {
	_, err := New(WithVersion("not-a-version"), WithStoragePath(filepath.Join(t.TempDir(), "firstrun.json")))
	if err == nil {
		t.Fatal("expected error for invalid WithVersion")
	}
}

func TestNewRejectsInvalidBaseline(t *testing.T) {
	_, err := New(
		WithVersion("1.0.0"),
		WithBaselineVersion("not-a-version"),
		WithStoragePath(filepath.Join(t.TempDir(), "firstrun.json")),
	)
	if err == nil {
		t.Fatal("expected error for invalid WithBaselineVersion")
	}
}

// TestFreshVsUpgradeVsDowngrade covers the core detection matrix: a store
// with no stamp is Fresh; running once at 1.0.0 then constructing a new
// Service at a newer version detects Upgrade; at an older version detects
// Downgrade; at the same version detects Same.
func TestFreshVsUpgradeVsDowngrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firstrun.json")

	fresh, err := New(WithVersion("1.0.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	info, err := fresh.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Kind != Fresh {
		t.Fatalf("Kind = %v, want Fresh", info.Kind)
	}
	if info.Previous.String() != "v0.0.0" {
		t.Fatalf("Previous = %v, want the zero Version", info.Previous)
	}

	if _, err := fresh.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	upgrade, err := New(WithVersion("1.1.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	info, err = upgrade.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Kind != Upgrade {
		t.Fatalf("Kind = %v, want Upgrade", info.Kind)
	}
	if info.Previous.String() != "v1.0.0" {
		t.Fatalf("Previous = %v, want v1.0.0", info.Previous)
	}
	if info.Current.String() != "v1.1.0" {
		t.Fatalf("Current = %v, want v1.1.0", info.Current)
	}

	same, err := New(WithVersion("1.0.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	info, err = same.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Kind != Same {
		t.Fatalf("Kind = %v, want Same", info.Kind)
	}

	downgrade, err := New(WithVersion("0.9.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	info, err = downgrade.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Kind != Downgrade {
		t.Fatalf("Kind = %v, want Downgrade", info.Kind)
	}
}

// TestStampOnlyAfterSuccess is the requirement-2 regression: a failing hook
// must leave the recorded version untouched so the next launch retries.
func TestStampOnlyAfterSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firstrun.json")
	boom := errors.New("boom")

	attempts := 0
	svc, err := New(
		WithVersion("1.0.0"),
		WithStoragePath(path),
		WithHooks(Hook{
			Name: "always-fails",
			Run: func(context.Context, Info) error {
				attempts++
				return boom
			},
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, runErr := svc.Run(context.Background())
	if runErr == nil {
		t.Fatal("expected Run to return the hook's error")
	}
	if !errors.Is(runErr, boom) {
		t.Fatalf("Run error does not unwrap to the hook's error: %v", runErr)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}

	// Detect must still report Fresh — nothing was recorded.
	info, err := svc.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Kind != Fresh {
		t.Fatalf("Kind = %v, want Fresh (stamp must not have been written)", info.Kind)
	}

	// Running again re-attempts the same (failing) hook rather than
	// skipping it as already-done.
	if _, err := svc.Run(context.Background()); err == nil {
		t.Fatal("expected second Run to fail again")
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2 (hook must re-run on retry)", attempts)
	}
}

// TestRunEmitsTransitionAfterSuccess checks EventTransition's payload and
// that emission happens without deadlocking a handler that calls back into
// the Service (i.e. emit is never called while s.mu is held).
func TestRunEmitsTransitionAfterSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firstrun.json")
	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	seed, err := New(WithVersion("1.0.0"), WithStoragePath(path), WithEmitter(emitter))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var reentrantErr error
	unsubscribe := events.On(emitter, EventTransition, func(TransitionPayload) {
		// A handler that calls back into Detect on the same Service must
		// not deadlock — this would hang (and the test would time out
		// under `go test`'s default deadline) if emit were called while
		// Run still held s.mu.
		_, reentrantErr = seed.Detect()
	})
	defer unsubscribe()

	info, err := seed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reentrantErr != nil {
		t.Fatalf("reentrant Detect from handler failed: %v", reentrantErr)
	}
	if info.Kind != Fresh {
		t.Fatalf("Kind = %v, want Fresh", info.Kind)
	}

	gotEvents := mem.Events()
	if len(gotEvents) != 1 {
		t.Fatalf("got %d events, want 1", len(gotEvents))
	}
	payload, ok := gotEvents[0].Data.(TransitionPayload)
	if !ok {
		t.Fatalf("event data is %T, want TransitionPayload", gotEvents[0].Data)
	}
	if payload.Kind != Fresh {
		t.Fatalf("payload.Kind = %v, want Fresh", payload.Kind)
	}
	if payload.Previous != "" {
		t.Fatalf("payload.Previous = %q, want \"\" for Fresh", payload.Previous)
	}
	if payload.Current != "v1.0.0" {
		t.Fatalf("payload.Current = %q, want v1.0.0", payload.Current)
	}
}

// TestRunDoesNotEmitOnFailure checks that a hook failure suppresses the
// transition event entirely — an app must not treat "I got
// firstrun:transition" as proof the upgrade completed if it can also fire
// on a run that actually failed.
func TestRunDoesNotEmitOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firstrun.json")
	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	svc, err := New(
		WithVersion("1.0.0"),
		WithStoragePath(path),
		WithEmitter(emitter),
		WithHooks(Hook{Name: "fails", Run: func(context.Context, Info) error {
			return errors.New("boom")
		}}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := svc.Run(context.Background()); err == nil {
		t.Fatal("expected Run to fail")
	}
	if mem.Count() != 0 {
		t.Fatalf("got %d events on a failed Run, want 0", mem.Count())
	}
}
