package kit

import (
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/v2/appearance"
	"github.com/jrschumacher/wails-kit/v2/events"
)

// fakeSource is a controllable appearance.Source test double.
type fakeSource struct {
	dark bool
	subs []func(bool)
}

func (f *fakeSource) IsDark() bool { return f.dark }

func (f *fakeSource) Subscribe(fn func(bool)) func() {
	f.subs = append(f.subs, fn)
	idx := len(f.subs) - 1
	return func() { f.subs[idx] = nil }
}

func (f *fakeSource) flip(dark bool) {
	f.dark = dark
	for _, fn := range f.subs {
		if fn != nil {
			fn(dark)
		}
	}
}

// TestAppearanceSeededWithOSSource pins that kit seeds the indirection with
// the platform's no-Wails Source rather than leaving it empty.
//
// The indirection exists so wailsbridge can retarget the Source after the
// App is constructed (see TestSetAppearanceSource). But a headless CLI or
// TUI never has an App, so an unseeded indirection would report light on
// macOS regardless of the real OS theme — in exactly the process shape kit
// exists to serve. Seeding costs nothing and wailsbridge can still replace it.
//
// The expected value is whatever the OS actually reports, so this asserts
// agreement with appearance.NewOSSource() rather than a hardcoded theme.
func TestAppearanceSeededWithOSSource(t *testing.T) {
	k := newTestKit(t)

	want := appearance.ThemeLight
	if src := appearance.NewOSSource(); src != nil && src.IsDark() {
		want = appearance.ThemeDark
	}

	if got := k.Appearance.Resolved(); got != want {
		t.Errorf("Resolved() = %q with no explicit Source, want %q (the OS source)", got, want)
	}
}

// TestAppearanceExplicitSourceWins pins that WithAppearanceSource still
// overrides the seeded OS source, including an explicit nil.
func TestAppearanceExplicitSourceWins(t *testing.T) {
	k := newTestKit(t, WithAppearanceSource(&fakeSource{dark: true}))

	if got := k.Appearance.Resolved(); got != appearance.ThemeDark {
		t.Errorf("Resolved() = %q with an explicit dark Source, want %q", got, appearance.ThemeDark)
	}
}

// TestSetAppearanceSource covers the wailsbridge seam: retargeting the
// Source after New returns actually changes what Resolved() reports, and a
// subsequent live change from that Source is delivered too (via
// dynamicSource.notify).
func TestSetAppearanceSource(t *testing.T) {
	k := newTestKit(t)
	fake := &fakeSource{dark: true}

	k.SetAppearanceSource(fake)

	if got := k.Appearance.Resolved(); got != appearance.ThemeDark {
		t.Fatalf("Resolved() = %q immediately after SetAppearanceSource(dark), want %q", got, appearance.ThemeDark)
	}

	mem := events.NewMemoryEmitter()
	k.SetEventsBackend(mem)
	fake.flip(false)

	if got := k.Appearance.Resolved(); got != appearance.ThemeLight {
		t.Errorf("Resolved() = %q after the wired Source flips to light, want %q", got, appearance.ThemeLight)
	}
	if !mem.WaitFor(appearance.EventChanged, time.Second) {
		t.Error("appearance:changed was not emitted after the Source flip")
	}
}

// TestWithAppearanceSourceSeedsAtConstruction covers WithAppearanceSource
// as a New-time equivalent to calling SetAppearanceSource immediately
// afterward.
func TestWithAppearanceSourceSeedsAtConstruction(t *testing.T) {
	fake := &fakeSource{dark: true}
	k := newTestKit(t, WithAppearanceSource(fake))

	if got := k.Appearance.Resolved(); got != appearance.ThemeDark {
		t.Errorf("Resolved() = %q, want %q", got, appearance.ThemeDark)
	}
}
