package windowstate

import "testing"

// TestManageRequiresApp: Manage's nil-app guard, exercised without
// constructing a real *application.App (which needs a live session to be
// useful and is a process-wide singleton per wails/v3 — see shortcuts'
// AGENTS.md landmines section). The win==nil guard is symmetric and
// trivial; it is not separately exercised here for the same "don't spin up
// a real App just to hit a three-line nil check" reason.
func TestManageRequiresApp(t *testing.T) {
	m, err := Manage(nil, nil)
	if err == nil {
		t.Fatal("expected an error when app is nil")
	}
	if m != nil {
		t.Error("expected a nil Manager on error")
	}
}

// TestNewManagerRequiresWindowAndScreens covers the same nil-guard shape at
// the Wails-free constructor level, which is fully testable without any
// Wails dependency at all.
func TestNewManagerRequiresWindowAndScreens(t *testing.T) {
	screens := singleScreen1080p()
	win := newFakeWindow(0, 0, 100, 100)

	if _, err := newManager(nil, screens, ""); err == nil {
		t.Error("expected an error when window is nil")
	}
	if _, err := newManager(win, nil, ""); err == nil {
		t.Error("expected an error when screen source is nil")
	}
}
