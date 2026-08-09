package windowstate

import "github.com/wailsapp/wails/v3/pkg/application"

// clamp resolves saved geometry g against the currently attached screens.
// If at least half of g's area overlaps some screen's WorkArea, g is
// returned unchanged (clamped=false) — the window is still substantially
// on an attached display, even if it hangs slightly off one edge. Otherwise
// (most commonly: the screen it was saved on is now detached, so overlap is
// zero) g is re-centered on the primary screen at its saved size, clamped
// to that screen's work area (clamped=true).
//
// This is pure and Wails-free of *behavior* (no window/app calls), but
// takes application.Screen directly rather than a hand-rolled struct: it's
// plain data (no display or cgo required to construct one), and windowstate
// already sits on the Wails-import allowlist, so there's no insulation to
// buy by reinventing the type.
func clamp(g Geometry, screens []*application.Screen) (Geometry, bool) {
	if len(screens) == 0 {
		// No screen info available at all — nothing sensible to clamp
		// against. Leave the saved geometry as-is rather than guessing.
		return g, false
	}

	saved := application.Rect{X: g.X, Y: g.Y, Width: g.W, Height: g.H}
	savedArea := rectArea(saved)

	best := 0
	for _, s := range screens {
		if s == nil {
			continue
		}
		if ov := intersectArea(saved, s.WorkArea); ov > best {
			best = ov
		}
	}

	if savedArea > 0 && best*2 >= savedArea {
		return g, false
	}

	primary := primaryScreen(screens)
	x, y, w, h := centerOn(primary, g.W, g.H)
	return Geometry{X: x, Y: y, W: w, H: h, Maximised: g.Maximised, ScreenID: primary.ID}, true
}

func rectArea(r application.Rect) int {
	if r.Width <= 0 || r.Height <= 0 {
		return 0
	}
	return r.Width * r.Height
}

// intersectArea returns the overlapping area of two rects, or 0 if they
// don't intersect.
func intersectArea(a, b application.Rect) int {
	x1, y1 := max(a.X, b.X), max(a.Y, b.Y)
	x2, y2 := min(a.X+a.Width, b.X+b.Width), min(a.Y+a.Height, b.Y+b.Height)
	if x2 <= x1 || y2 <= y1 {
		return 0
	}
	return (x2 - x1) * (y2 - y1)
}

// primaryScreen returns the screen with IsPrimary set, falling back to the
// first screen in the slice if none is marked primary (defensive — the real
// ScreenManager always designates one, but a fake in tests might not).
func primaryScreen(screens []*application.Screen) *application.Screen {
	for _, s := range screens {
		if s != nil && s.IsPrimary {
			return s
		}
	}
	return screens[0]
}

// centerOn centers a w x h window on screen's work area, clamping the size
// down to fit if it's larger than the work area (and treating a
// non-positive requested dimension as "use the whole work area dimension").
func centerOn(screen *application.Screen, w, h int) (x, y, cw, ch int) {
	wa := screen.WorkArea
	cw, ch = w, h
	if cw <= 0 || cw > wa.Width {
		cw = wa.Width
	}
	if ch <= 0 || ch > wa.Height {
		ch = wa.Height
	}
	x = wa.X + (wa.Width-cw)/2
	y = wa.Y + (wa.Height-ch)/2
	return x, y, cw, ch
}
