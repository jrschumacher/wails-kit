package lifecycle

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
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
