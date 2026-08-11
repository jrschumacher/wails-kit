package lifecycle

import (
	"context"
	stderrors "errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
)

// mockService tracks startup/shutdown calls and can be configured to fail.
type mockService struct {
	started    bool
	stopped    bool
	startErr   error
	stopErr    error
	startOrder *[]string
	name       string
}

func (m *mockService) OnStartup(_ context.Context) error {
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	if m.startOrder != nil {
		*m.startOrder = append(*m.startOrder, m.name)
	}
	return nil
}

func (m *mockService) OnShutdown() error {
	if m.stopErr != nil {
		return m.stopErr
	}
	m.stopped = true
	return nil
}

// slowService blocks for a duration on startup and/or shutdown.
type slowService struct {
	startDelay time.Duration
	stopDelay  time.Duration
}

func (s *slowService) OnStartup(ctx context.Context) error {
	select {
	case <-time.After(s.startDelay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *slowService) OnShutdown() error {
	time.Sleep(s.stopDelay)
	return nil
}

// healthyService implements HealthChecker and always returns a fixed status.
type healthyService struct {
	mockService
	status HealthStatus
}

func (h *healthyService) Health() HealthStatus {
	return h.status
}

// ignoresContextService ignores context cancellation during OnStartup —
// it always sleeps out startDelay and reports success, regardless of
// whether the manager already gave up on it. Used to exercise the
// timed-out-but-later-succeeds reap path.
type ignoresContextService struct {
	startDelay time.Duration
	started    chan struct{}
	stopped    chan struct{}
}

func (s *ignoresContextService) OnStartup(_ context.Context) error {
	time.Sleep(s.startDelay)
	close(s.started)
	return nil
}

func (s *ignoresContextService) OnShutdown() error {
	close(s.stopped)
	return nil
}

func TestNewManager_NoDeps(t *testing.T) {
	a := &mockService{name: "a"}
	b := &mockService{name: "b"}

	mgr, err := NewManager(
		WithService("a", a),
		WithService("b", b),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mgr.Order()) != 2 {
		t.Fatalf("expected 2 services, got %d", len(mgr.Order()))
	}
}

func TestNewManager_DependencyOrder(t *testing.T) {
	var order []string
	db := &mockService{name: "db", startOrder: &order}
	settings := &mockService{name: "settings", startOrder: &order}
	updates := &mockService{name: "updates", startOrder: &order}

	mgr, err := NewManager(
		WithService("updates", updates, DependsOn("settings")),
		WithService("settings", settings, DependsOn("db")),
		WithService("db", db),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"db", "settings", "updates"}
	got := mgr.Order()
	for i, name := range expected {
		if got[i] != name {
			t.Fatalf("expected order %v, got %v", expected, got)
		}
	}
}

func TestNewManager_MissingDependency(t *testing.T) {
	a := &mockService{name: "a"}

	_, err := NewManager(
		WithService("a", a, DependsOn("missing")),
	)
	if err == nil {
		t.Fatal("expected error for missing dependency")
	}
	if !errors.IsCode(err, ErrMissingDep) {
		t.Fatalf("expected ErrMissingDep, got %v", err)
	}
}

func TestNewManager_CyclicDependency(t *testing.T) {
	a := &mockService{name: "a"}
	b := &mockService{name: "b"}

	_, err := NewManager(
		WithService("a", a, DependsOn("b")),
		WithService("b", b, DependsOn("a")),
	)
	if err == nil {
		t.Fatal("expected error for cyclic dependency")
	}
	if !errors.IsCode(err, ErrCyclicDependency) {
		t.Fatalf("expected ErrCyclicDependency, got %v", err)
	}
}

func TestStartup_Success(t *testing.T) {
	var order []string
	db := &mockService{name: "db", startOrder: &order}
	settings := &mockService{name: "settings", startOrder: &order}

	mem := events.NewMemoryEmitter()
	mgr, err := NewManager(
		WithService("settings", settings, DependsOn("db")),
		WithService("db", db),
		WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.Startup(context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	// Verify start order.
	if len(order) != 2 || order[0] != "db" || order[1] != "settings" {
		t.Fatalf("expected start order [db settings], got %v", order)
	}

	// Verify events.
	evts := mem.Events()
	if len(evts) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evts))
	}
	if evts[0].Name != EventStarted {
		t.Fatalf("expected %s event, got %s", EventStarted, evts[0].Name)
	}
}

func TestShutdown_ReverseOrder(t *testing.T) {
	var startOrder []string
	db := &mockService{name: "db", startOrder: &startOrder}
	settings := &mockService{name: "settings", startOrder: &startOrder}
	updates := &mockService{name: "updates", startOrder: &startOrder}

	mem := events.NewMemoryEmitter()
	mgr, err := NewManager(
		WithService("updates", updates, DependsOn("settings")),
		WithService("settings", settings, DependsOn("db")),
		WithService("db", db),
		WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.Startup(context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	mem.Clear()

	if err := mgr.Shutdown(); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	// All services should be stopped.
	if !db.stopped || !settings.stopped || !updates.stopped {
		t.Fatal("not all services were stopped")
	}

	// Verify stopped events in reverse order.
	evts := mem.Events()
	if len(evts) != 3 {
		t.Fatalf("expected 3 events, got %d", len(evts))
	}
	names := []string{
		evts[0].Data.(ServiceStoppedPayload).Name,
		evts[1].Data.(ServiceStoppedPayload).Name,
		evts[2].Data.(ServiceStoppedPayload).Name,
	}
	expected := []string{"updates", "settings", "db"}
	for i, name := range expected {
		if names[i] != name {
			t.Fatalf("expected shutdown order %v, got %v", expected, names)
		}
	}
}

func TestStartup_PartialFailureRollback(t *testing.T) {
	var startOrder []string
	db := &mockService{name: "db", startOrder: &startOrder}
	settings := &mockService{name: "settings", startErr: stderrors.New("settings init failed"), startOrder: &startOrder}
	updates := &mockService{name: "updates", startOrder: &startOrder}

	mgr, err := NewManager(
		WithService("updates", updates, DependsOn("settings")),
		WithService("settings", settings, DependsOn("db")),
		WithService("db", db),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = mgr.Startup(context.Background())
	if err == nil {
		t.Fatal("expected startup error")
	}

	if !errors.IsCode(err, ErrStartup) {
		t.Fatalf("expected ErrStartup, got %v", err)
	}

	// db should have been started then rolled back.
	if !db.stopped {
		t.Fatal("db should have been rolled back (stopped)")
	}

	// settings never started successfully; updates never started.
	if settings.started {
		t.Fatal("settings should not be marked as started")
	}
	if updates.started {
		t.Fatal("updates should not have started")
	}
}

func TestShutdown_CollectsAllErrors(t *testing.T) {
	db := &mockService{name: "db", stopErr: stderrors.New("db stop failed")}
	settings := &mockService{name: "settings", stopErr: stderrors.New("settings stop failed")}

	mgr, err := NewManager(
		WithService("settings", settings, DependsOn("db")),
		WithService("db", db),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.Startup(context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	err = mgr.Shutdown()
	if err == nil {
		t.Fatal("expected shutdown error")
	}

	// Both errors should be present.
	msg := err.Error()
	if !strings.Contains(msg, "settings") || !strings.Contains(msg, "db") {
		t.Fatalf("expected both service errors, got: %s", msg)
	}
}

func TestRollback_WithShutdownErrors(t *testing.T) {
	db := &mockService{name: "db", stopErr: stderrors.New("db won't stop")}
	settings := &mockService{name: "settings", startErr: stderrors.New("settings broken")}

	mem := events.NewMemoryEmitter()
	mgr, err := NewManager(
		WithService("settings", settings, DependsOn("db")),
		WithService("db", db),
		WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = mgr.Startup(context.Background())
	if err == nil {
		t.Fatal("expected startup error")
	}

	// Should contain both the startup error and the rollback error.
	msg := err.Error()
	if !strings.Contains(msg, "settings") {
		t.Fatalf("expected startup error for settings, got: %s", msg)
	}
	if !strings.Contains(msg, "db won't stop") {
		t.Fatalf("expected rollback error for db, got: %s", msg)
	}

	// Verify rollback event was emitted.
	var foundRollback bool
	for _, evt := range mem.Events() {
		if evt.Name == EventRollback {
			foundRollback = true
			payload := evt.Data.(RollbackPayload)
			if payload.FailedService != "settings" {
				t.Fatalf("expected failed service 'settings', got %q", payload.FailedService)
			}
		}
	}
	if !foundRollback {
		t.Fatal("expected rollback event")
	}
}

func TestStartup_NoServices(t *testing.T) {
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mgr.Startup(context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := mgr.Shutdown(); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func TestDiamondDependency(t *testing.T) {
	// A depends on B and C; B and C both depend on D.
	var order []string
	d := &mockService{name: "d", startOrder: &order}
	b := &mockService{name: "b", startOrder: &order}
	c := &mockService{name: "c", startOrder: &order}
	a := &mockService{name: "a", startOrder: &order}

	mgr, err := NewManager(
		WithService("a", a, DependsOn("b", "c")),
		WithService("b", b, DependsOn("d")),
		WithService("c", c, DependsOn("d")),
		WithService("d", d),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sorted := mgr.Order()
	// d must come before b and c; b and c must come before a.
	indexOf := func(name string) int {
		for i, n := range sorted {
			if n == name {
				return i
			}
		}
		return -1
	}

	if indexOf("d") >= indexOf("b") || indexOf("d") >= indexOf("c") {
		t.Fatalf("d must come before b and c, got %v", sorted)
	}
	if indexOf("b") >= indexOf("a") || indexOf("c") >= indexOf("a") {
		t.Fatalf("b and c must come before a, got %v", sorted)
	}
}

// --- Timeout tests ---

func TestStartup_GlobalTimeout_Triggers(t *testing.T) {
	slow := &slowService{startDelay: 500 * time.Millisecond}

	mem := events.NewMemoryEmitter()
	mgr, err := NewManager(
		WithService("slow", slow),
		WithTimeout(50*time.Millisecond),
		WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = mgr.Startup(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.IsCode(err, ErrStartup) {
		t.Fatalf("expected ErrStartup wrapping timeout, got %v", err)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout message, got: %s", err.Error())
	}

	// Verify timeout event was emitted.
	var foundTimeout bool
	for _, evt := range mem.Events() {
		if evt.Name == EventTimeout {
			foundTimeout = true
			payload := evt.Data.(TimeoutPayload)
			if payload.Name != "slow" || payload.Phase != "startup" {
				t.Fatalf("unexpected timeout payload: %+v", payload)
			}
		}
	}
	if !foundTimeout {
		t.Fatal("expected timeout event")
	}
}

func TestStartup_GlobalTimeout_NoTimeoutWhenFast(t *testing.T) {
	fast := &mockService{name: "fast"}

	mgr, err := NewManager(
		WithService("fast", fast),
		WithTimeout(1*time.Second),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.Startup(context.Background()); err != nil {
		t.Fatalf("startup should succeed: %v", err)
	}
	if !fast.started {
		t.Fatal("service should have started")
	}
}

func TestStartup_PerServiceTimeout_Overrides(t *testing.T) {
	// Global timeout is generous, but per-service timeout is short.
	slow := &slowService{startDelay: 500 * time.Millisecond}

	mgr, err := NewManager(
		WithService("slow", slow, WithServiceTimeout(50*time.Millisecond)),
		WithTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = mgr.Startup(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout message, got: %s", err.Error())
	}
}

func TestShutdown_Timeout_ContinuesOtherServices(t *testing.T) {
	fast := &mockService{name: "fast"}
	slow := &slowService{stopDelay: 500 * time.Millisecond}

	mem := events.NewMemoryEmitter()
	mgr, err := NewManager(
		WithService("slow", slow),
		WithService("fast", fast, DependsOn("slow")),
		WithTimeout(50*time.Millisecond),
		WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.Startup(context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	mem.Clear()

	err = mgr.Shutdown()
	if err == nil {
		t.Fatal("expected shutdown error from timeout")
	}
	// The fast service should have stopped successfully despite slow timing out.
	if !fast.stopped {
		t.Fatal("fast service should have been stopped")
	}
}

func TestStartup_TimeoutTriggersRollback(t *testing.T) {
	fast := &mockService{name: "fast"}
	slow := &slowService{startDelay: 500 * time.Millisecond}

	mgr, err := NewManager(
		WithService("fast", fast),
		WithService("slow", slow, DependsOn("fast"), WithServiceTimeout(50*time.Millisecond)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = mgr.Startup(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}

	// fast should have been rolled back.
	if !fast.stopped {
		t.Fatal("fast service should have been rolled back")
	}
}

// --- Health check tests ---

func TestHealth_AllHealthy(t *testing.T) {
	a := &mockService{name: "a"}
	b := &mockService{name: "b"}

	mgr, err := NewManager(
		WithService("a", a),
		WithService("b", b),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mgr.Startup(context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	health := mgr.Health()
	if len(health) != 2 {
		t.Fatalf("expected 2 health entries, got %d", len(health))
	}
	for _, h := range health {
		if h.Status != StatusHealthy {
			t.Fatalf("expected healthy, got %s for %s", h.Status, h.Name)
		}
	}
}

func TestHealth_WithHealthChecker(t *testing.T) {
	a := &mockService{name: "a"}
	b := &healthyService{
		mockService: mockService{name: "b"},
		status:      StatusDegraded,
	}

	mgr, err := NewManager(
		WithService("a", a),
		WithService("b", b),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mgr.Startup(context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	health := mgr.Health()
	statusByName := make(map[string]HealthStatus)
	for _, h := range health {
		statusByName[h.Name] = h.Status
	}

	if statusByName["a"] != StatusHealthy {
		t.Fatalf("expected a=healthy, got %s", statusByName["a"])
	}
	if statusByName["b"] != StatusDegraded {
		t.Fatalf("expected b=degraded, got %s", statusByName["b"])
	}
}

func TestHealth_EmptyBeforeStartup(t *testing.T) {
	mgr, err := NewManager(
		WithService("a", &mockService{name: "a"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	health := mgr.Health()
	if len(health) != 0 {
		t.Fatalf("expected 0 health entries before startup, got %d", len(health))
	}
}

func TestHealth_Unhealthy(t *testing.T) {
	svc := &healthyService{
		mockService: mockService{name: "db"},
		status:      StatusUnhealthy,
	}

	mgr, err := NewManager(WithService("db", svc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mgr.Startup(context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	health := mgr.Health()
	if len(health) != 1 || health[0].Status != StatusUnhealthy {
		t.Fatalf("expected unhealthy, got %+v", health)
	}
}

// --- Regression tests: duplicate names, rollback timeouts, concurrency,
// and orphaned timed-out startups ---

// TestDuplicateServiceName is the regression test for NewManager silently
// accepting two services registered under the same name. topoSort used to
// dedupe names into a set for its cycle check, so len(order) still ended up
// looking consistent, and entryMap() is last-writer-wins — so one entry
// silently vanishes, its service is never started, and whichever entry
// wins the map gets started (and later shut down) as if it were both.
func TestDuplicateServiceName(t *testing.T) {
	var order []string
	a1 := &mockService{name: "a1", startOrder: &order}
	a2 := &mockService{name: "a2", startOrder: &order}

	_, err := NewManager(
		WithService("a", a1),
		WithService("a", a2),
	)
	if err == nil {
		t.Fatal("expected error for duplicate service name")
	}
	if !errors.IsCode(err, ErrDuplicateService) {
		t.Fatalf("expected ErrDuplicateService, got %v", err)
	}
}

// TestRollbackHonorsTimeout is the regression test for rollback calling
// OnShutdown directly instead of going through stopService: a service that
// hangs in OnShutdown during rollback used to block Startup forever, even
// with a timeout configured specifically to prevent that. "Forever" isn't
// something a test can wait out, so this asserts Startup returns well
// within the configured timeout instead of hanging — a service with a
// 500ms shutdown delay against a 50ms service timeout must not make the
// test (or a real Startup call) wait 500ms.
func TestRollbackHonorsTimeout(t *testing.T) {
	db := &slowService{stopDelay: 500 * time.Millisecond}
	settings := &mockService{name: "settings", startErr: stderrors.New("settings broken")}

	mem := events.NewMemoryEmitter()
	mgr, err := NewManager(
		WithService("settings", settings, DependsOn("db")),
		WithService("db", db, WithServiceTimeout(50*time.Millisecond)),
		WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- mgr.Startup(context.Background()) }()

	select {
	case err = <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Startup did not return within 300ms — rollback did not honor the configured per-service timeout")
	}

	if err == nil {
		t.Fatal("expected startup error")
	}

	// A shutdown timeout event must have been emitted for db during rollback.
	var foundTimeout bool
	for _, evt := range mem.Events() {
		if evt.Name == EventTimeout {
			payload := evt.Data.(TimeoutPayload)
			if payload.Name == "db" && payload.Phase == "shutdown" {
				foundTimeout = true
			}
		}
	}
	if !foundTimeout {
		t.Fatal("expected a shutdown timeout event emitted during rollback")
	}
}

// TestStartup_TwiceWithoutShutdown_ReturnsError is the regression test for
// "Startup twice double-starts everything": calling Startup a second time
// without an intervening Shutdown used to reset m.started and re-run every
// service's OnStartup from scratch, on top of whatever was already running.
func TestStartup_TwiceWithoutShutdown_ReturnsError(t *testing.T) {
	var order []string
	a := &mockService{name: "a", startOrder: &order}

	mgr, err := NewManager(WithService("a", a))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.Startup(context.Background()); err != nil {
		t.Fatalf("first startup failed: %v", err)
	}

	err = mgr.Startup(context.Background())
	if err == nil {
		t.Fatal("expected an error calling Startup twice without an intervening Shutdown")
	}
	if !errors.IsCode(err, ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState, got %v", err)
	}

	if len(order) != 1 {
		t.Fatalf("service must start exactly once, got %d starts: %v", len(order), order)
	}
}

// TestStartup_ConcurrentCalls_OnlyOneSucceeds exercises the "no mutex, not
// safe for concurrent use" gap directly: two goroutines racing Startup()
// must not both succeed (double-starting the service) and must not race on
// m.started — run with -race.
func TestStartup_ConcurrentCalls_OnlyOneSucceeds(t *testing.T) {
	a := &slowService{startDelay: 50 * time.Millisecond}

	mgr, err := NewManager(WithService("a", a))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			errCh <- mgr.Startup(context.Background())
		}()
	}
	wg.Wait()
	close(errCh)

	var successCount, invalidStateCount int
	for err := range errCh {
		switch {
		case err == nil:
			successCount++
		case errors.IsCode(err, ErrInvalidState):
			invalidStateCount++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful concurrent Startup, got %d", successCount)
	}
	if invalidStateCount != 1 {
		t.Fatalf("expected exactly 1 ErrInvalidState from the losing concurrent Startup, got %d", invalidStateCount)
	}
}

// TestShutdown_SafeNoOpWhenNeverStarted verifies Shutdown can be called
// unconditionally (the common `defer mgr.Shutdown()` pattern) even when
// Startup was never called or already failed, without panicking or acting
// on stale state.
func TestShutdown_SafeNoOpWhenNeverStarted(t *testing.T) {
	mgr, err := NewManager(WithService("a", &mockService{name: "a"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.Shutdown(); err != nil {
		t.Fatalf("Shutdown on a never-started manager should be a no-op, got: %v", err)
	}
}

// TestStartup_TimeoutReapsLateSuccess is the regression test for a startup
// service that times out but ignores context cancellation and eventually
// succeeds anyway: it must not be orphaned (leaked, still running, with no
// caller ever able to shut it down again since it's not in m.started).
func TestStartup_TimeoutReapsLateSuccess(t *testing.T) {
	svc := &ignoresContextService{
		startDelay: 100 * time.Millisecond,
		started:    make(chan struct{}),
		stopped:    make(chan struct{}),
	}

	mgr, err := NewManager(
		WithService("slow", svc, WithServiceTimeout(20*time.Millisecond)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = mgr.Startup(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}

	select {
	case <-svc.started:
	case <-time.After(1 * time.Second):
		t.Fatal("service never finished its (ignored-timeout) OnStartup call")
	}

	select {
	case <-svc.stopped:
	case <-time.After(1 * time.Second):
		t.Fatal("service that timed out but later succeeded was never shut down (orphaned)")
	}
}
