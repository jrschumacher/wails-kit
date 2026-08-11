package wailsbridge

import "testing"

// TestThemeSource_IsDark exercises the deterministic path: a headless test
// app never calls Run(), so app.impl stays nil and
// EnvironmentManager.IsDarkMode() (wails/v3 environment_manager.go) returns
// false unconditionally — this is the one path in themeSource that's
// testable without a real display/OS session. See AGENTS.md, "Testing" for
// what isn't (Subscribe's OS-driven callback).
func TestThemeSource_IsDark(t *testing.T) {
	app := testApp(t)
	src := newThemeSource(app)

	if got := src.IsDark(); got != false {
		t.Errorf("IsDark() = %v, want false for a headless (never Run) app", got)
	}
}

// TestThemeSource_SubscribeReturnsCancel verifies Subscribe registers
// without error and returns a usable, idempotent cancel func — the part of
// app.Event.OnApplicationEvent's contract that doesn't require a real
// OS theme-change notification to exercise.
func TestThemeSource_SubscribeReturnsCancel(t *testing.T) {
	app := testApp(t)
	src := newThemeSource(app)

	cancel := src.Subscribe(func(dark bool) {})
	if cancel == nil {
		t.Fatal("expected a non-nil cancel func")
	}
	cancel()
	cancel() // must not panic on a second call
}
