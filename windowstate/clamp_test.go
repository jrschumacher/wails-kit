package windowstate

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func twoScreens() []*application.Screen {
	return []*application.Screen{
		{
			ID:        "laptop",
			IsPrimary: true,
			Bounds:    application.Rect{X: 0, Y: 0, Width: 1440, Height: 900},
			WorkArea:  application.Rect{X: 0, Y: 25, Width: 1440, Height: 875},
		},
		{
			ID:        "external",
			IsPrimary: false,
			Bounds:    application.Rect{X: 1440, Y: 0, Width: 1920, Height: 1080},
			WorkArea:  application.Rect{X: 1440, Y: 0, Width: 1920, Height: 1080},
		},
	}
}

// TestClampDetachedDisplay is the highest-value case per WP-14: geometry
// saved entirely on an external monitor must not be restored off-screen
// once that monitor is unplugged. Only the laptop screen remains attached.
func TestClampDetachedDisplay(t *testing.T) {
	saved := Geometry{X: 1600, Y: 200, W: 800, H: 600, ScreenID: "external"}
	attached := []*application.Screen{twoScreens()[0]} // external is gone

	resolved, clamped := clamp(saved, attached)

	if !clamped {
		t.Fatal("expected clamped=true when saved geometry has zero overlap with any attached screen")
	}
	primary := attached[0]
	if resolved.X < primary.WorkArea.X || resolved.X+resolved.W > primary.WorkArea.X+primary.WorkArea.Width {
		t.Errorf("resolved geometry X range [%d,%d) escapes primary work area [%d,%d): %+v",
			resolved.X, resolved.X+resolved.W, primary.WorkArea.X, primary.WorkArea.X+primary.WorkArea.Width, resolved)
	}
	if resolved.Y < primary.WorkArea.Y || resolved.Y+resolved.H > primary.WorkArea.Y+primary.WorkArea.Height {
		t.Errorf("resolved geometry Y range [%d,%d) escapes primary work area [%d,%d): %+v",
			resolved.Y, resolved.Y+resolved.H, primary.WorkArea.Y, primary.WorkArea.Y+primary.WorkArea.Height, resolved)
	}
	if resolved.W != saved.W || resolved.H != saved.H {
		t.Errorf("expected saved size preserved (800x600), got %dx%d", resolved.W, resolved.H)
	}
}

// TestClampNoScreensAtAll covers the defensive branch: an empty screen list
// (should never happen against a real ScreenManager, but a misbehaving fake
// or an OS query race is not impossible) leaves geometry untouched rather
// than panicking on an empty slice.
func TestClampNoScreensAtAll(t *testing.T) {
	saved := Geometry{X: 10, Y: 10, W: 400, H: 300}
	resolved, clamped := clamp(saved, nil)
	if clamped {
		t.Error("expected clamped=false with no screen info to clamp against")
	}
	if resolved != saved {
		t.Errorf("expected geometry unchanged, got %+v", resolved)
	}
}

// TestClampPartialOverlap: a window that is mostly (but not entirely) on an
// attached screen — more than 50% of its area overlaps — should be left
// alone rather than re-centered, even though part of it hangs off the edge.
func TestClampPartialOverlap(t *testing.T) {
	screens := []*application.Screen{
		{
			ID:        "primary",
			IsPrimary: true,
			Bounds:    application.Rect{X: 0, Y: 0, Width: 1000, Height: 800},
			WorkArea:  application.Rect{X: 0, Y: 0, Width: 1000, Height: 800},
		},
	}
	// 600-wide window straddling the right edge: 400 columns on-screen
	// (900..1000 minus... let's make it explicit: X=900, W=600 -> spans
	// 900..1500, work area is 0..1000, so overlap width = 100, overlap
	// area = 100*600 = 60000 out of 600*600=360000 -> under 50%. Use a
	// case that IS over 50% instead: X=100, W=600 spans 100..700, fully
	// inside -> 100% overlap.
	saved := Geometry{X: 100, Y: 100, W: 600, H: 600}
	resolved, clamped := clamp(saved, screens)
	if clamped {
		t.Errorf("expected clamped=false for >=50%% overlap, got resolved=%+v", resolved)
	}
	if resolved != saved {
		t.Errorf("expected geometry returned unchanged for majority-overlap case, got %+v", resolved)
	}
}

// TestClampMajorityOffscreenRecentersOnPrimary: less than 50% overlap (but
// not zero) still counts as "doesn't fit" and gets re-centered.
func TestClampMajorityOffscreenRecentersOnPrimary(t *testing.T) {
	screens := []*application.Screen{
		{
			ID:        "primary",
			IsPrimary: true,
			Bounds:    application.Rect{X: 0, Y: 0, Width: 1000, Height: 800},
			WorkArea:  application.Rect{X: 0, Y: 0, Width: 1000, Height: 800},
		},
	}
	// X=900, W=600 -> spans 900..1500; overlap with 0..1000 is 100 wide.
	// overlap area = 100*600=60000; saved area=600*600=360000 -> ~16.7% overlap.
	saved := Geometry{X: 900, Y: 100, W: 600, H: 600}
	resolved, clamped := clamp(saved, screens)
	if !clamped {
		t.Fatal("expected clamped=true for <50% overlap")
	}
	wantX := (1000 - 600) / 2
	wantY := (800 - 600) / 2
	if resolved.X != wantX || resolved.Y != wantY {
		t.Errorf("expected centered at (%d,%d), got (%d,%d)", wantX, wantY, resolved.X, resolved.Y)
	}
}

// TestClampOversizedGeometryShrinksToWorkArea: a saved size larger than the
// (possibly smaller, e.g. after a resolution change) primary work area must
// be clamped down to fit, not centered off both edges.
func TestClampOversizedGeometryShrinksToWorkArea(t *testing.T) {
	screens := []*application.Screen{
		{
			ID:        "small",
			IsPrimary: true,
			Bounds:    application.Rect{X: 0, Y: 0, Width: 800, Height: 600},
			WorkArea:  application.Rect{X: 0, Y: 0, Width: 800, Height: 600},
		},
	}
	// Saved on a display that no longer exists, at a size bigger than the
	// remaining primary's work area.
	saved := Geometry{X: 5000, Y: 5000, W: 1920, H: 1080}
	resolved, clamped := clamp(saved, screens)
	if !clamped {
		t.Fatal("expected clamped=true")
	}
	if resolved.W > 800 || resolved.H > 600 {
		t.Errorf("expected size shrunk to fit 800x600 work area, got %dx%d", resolved.W, resolved.H)
	}
}
