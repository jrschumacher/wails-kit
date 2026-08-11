package firstrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// TestDetectRecoversFromCorruptStamp is the regression test for M4a: before
// the fix, a corrupt stamp file made state.Store.Load return an error,
// which Detect/Run propagated on every single launch with no way out short
// of a user finding the (internal, undocumented-to-them) stamp path and
// deleting it by hand. Detect must instead recover: report Kind == Same
// (not Fresh — see detect's comment for why re-running onboarding would be
// worse) and leave the corrupt bytes preserved in a quarantine file rather
// than destroying them.
func TestDetectRecoversFromCorruptStamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firstrun.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	svc, err := New(WithVersion("1.4.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	info, err := svc.Detect()
	if err != nil {
		t.Fatalf("Detect returned an error for a corrupt stamp, want recovery: %v", err)
	}
	if info.Kind != Same {
		t.Fatalf("Kind = %v, want Same (corruption is a distinct outcome, not Fresh)", info.Kind)
	}
	if info.Current.String() != "v1.4.0" || info.Previous.String() != "v1.4.0" {
		t.Fatalf("info = %+v, want Previous == Current == v1.4.0", info)
	}

	// The original stamp file's bytes must be preserved, not discarded.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected corrupt stamp to be moved aside, stat err = %v", err)
	}
	matches, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one quarantined stamp file, got %v", matches)
	}
}

// TestRunRecoversFromCorruptStampWithoutRerunningOnboarding is the
// requirement-M4a regression covering the interaction the fix must get
// right: a corrupt stamp must not be treated as "no stamp", or an existing
// user would see OnFresh onboarding hooks re-run. It also checks the other
// half — Run must self-heal by writing a valid stamp, so recovery is a
// one-launch event, not a recurring one.
func TestRunRecoversFromCorruptStampWithoutRerunningOnboarding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firstrun.json")
	if err := os.WriteFile(path, []byte("{ not valid json"), 0600); err != nil {
		t.Fatal(err)
	}

	var onFreshRan, alwaysRan bool
	svc, err := New(
		WithVersion("1.4.0"),
		WithStoragePath(path),
		WithHooks(
			Hook{Name: "onboarding", When: OnFresh(), Run: func(context.Context, Info) error {
				onFreshRan = true
				return nil
			}},
			Hook{Name: "always", Run: func(context.Context, Info) error {
				alwaysRan = true
				return nil
			}},
		),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	info, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned an error for a corrupt stamp, want recovery: %v", err)
	}
	if info.Kind != Same {
		t.Fatalf("Kind = %v, want Same", info.Kind)
	}
	if onFreshRan {
		t.Fatal("OnFresh hook ran on corruption-recovery — this would re-run onboarding for an existing user")
	}
	if !alwaysRan {
		t.Fatal("an unfiltered (When == nil) hook must still run on corruption-recovery")
	}

	// Recovery must be self-healing: the next launch sees a normal, valid
	// stamp and reports Same again (a genuine relaunch), not another
	// recovery.
	relaunch, err := New(WithVersion("1.4.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	info, err = relaunch.Detect()
	if err != nil {
		t.Fatalf("Detect after recovery: %v", err)
	}
	if info.Kind != Same {
		t.Fatalf("Kind after recovery = %v, want Same", info.Kind)
	}
	matches, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one quarantined stamp file (no new one from the relaunch), got %v", matches)
	}
}

// TestReset is the regression/coverage test for the "no Reset()" half of
// M4a: an app must be able to force the next Detect/Run to treat the
// install as if it had never run, without knowing where the stamp file
// lives.
func TestReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firstrun.json")
	svc, err := New(WithVersion("1.0.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	info, err := svc.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Kind != Same {
		t.Fatalf("Kind before Reset = %v, want Same", info.Kind)
	}

	if err := svc.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	info, err = svc.Detect()
	if err != nil {
		t.Fatalf("Detect after Reset: %v", err)
	}
	if info.Kind != Fresh {
		t.Fatalf("Kind after Reset = %v, want Fresh", info.Kind)
	}
}

// TestHookReentryDoesNotDeadlock is the regression test for M4b: Run used
// to invoke every Hook's Run function while still holding s.mu (acquired in
// what was runLocked), so a hook that called back into Detect or Run on the
// same Service deadlocked forever — sync.Mutex is not re-entrant, and this
// is a same-goroutine self-deadlock (Run -> hook.Run -> Detect -> Lock),
// which Go does not detect or panic on; it just hangs. This test fails by
// timing out if the regression reappears, exactly like state's
// TestEmitOutsideLock and settings' equivalent.
func TestHookReentryDoesNotDeadlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firstrun.json")

	var svc *Service
	reentrantErrCh := make(chan error, 1)

	hook := Hook{
		Name: "reentrant",
		Run: func(context.Context, Info) error {
			// Reentrant call from within a hook's Run. If Run still held
			// s.mu while invoking hooks, this would deadlock the calling
			// goroutine against itself.
			_, err := svc.Detect()
			reentrantErrCh <- err
			return nil
		},
	}

	var err error
	svc, err = New(WithVersion("1.0.0"), WithStoragePath(path), WithHooks(hook))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	runDone := make(chan error, 1)
	go func() {
		_, runErr := svc.Run(context.Background())
		runDone <- runErr
	}()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned an error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run deadlocked (a hook was invoked while the service lock was held)")
	}

	select {
	case err := <-reentrantErrCh:
		if err != nil {
			t.Fatalf("reentrant Detect from the hook failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reentrant hook call deadlocked")
	}
}
