package i18n

import (
	"testing"

	"github.com/jrschumacher/wails-kit/v2/events"
)

// TestSetLocaleEmits covers SetLocale's contract: it emits EventChanged,
// transition-only (matching the kit's other *:changed events — e.g. health
// only emits on state transitions, not every tick).
func TestSetLocaleEmits(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	mem := events.NewMemoryEmitter()
	l, err := New(WithEmitter(events.NewEmitter(mem)))
	if err != nil {
		t.Fatal(err)
	}

	if err := l.SetLocale("fr"); err != nil {
		t.Fatal(err)
	}
	if mem.Count() != 1 {
		t.Fatalf("expected 1 event after a real change, got %d", mem.Count())
	}
	last := mem.Last()
	if last.Name != EventChanged {
		t.Errorf("event name = %q, want %q", last.Name, EventChanged)
	}
	payload, ok := last.Data.(ChangedPayload)
	if !ok || payload.Locale != "fr" {
		t.Errorf("event payload = %#v, want ChangedPayload{Locale: \"fr\"}", last.Data)
	}
	if got := l.Locale(); got != "fr" {
		t.Errorf("Locale() = %q, want %q", got, "fr")
	}

	// Setting the same locale again is not a transition.
	if err := l.SetLocale("fr"); err != nil {
		t.Fatal(err)
	}
	if mem.Count() != 1 {
		t.Errorf("expected no additional event for a no-op SetLocale, got %d total", mem.Count())
	}
}

func TestSetLocaleInvalidTag(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	l, err := New()
	if err != nil {
		t.Fatal(err)
	}
	before := l.Locale()
	if err := l.SetLocale("this is not a bcp47 tag!!"); err == nil {
		t.Error("SetLocale with an invalid tag should return an error")
	}
	if got := l.Locale(); got != before {
		t.Errorf("Locale() changed to %q after a rejected SetLocale, want unchanged %q", got, before)
	}
}

func TestSetLocaleNilEmitterIsSilent(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	l, err := New() // no WithEmitter
	if err != nil {
		t.Fatal(err)
	}
	if err := l.SetLocale("fr"); err != nil {
		t.Fatal(err)
	}
	if got := l.Locale(); got != "fr" {
		t.Errorf("Locale() = %q, want %q", got, "fr")
	}
}

func TestBinding(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	l, err := New()
	if err != nil {
		t.Fatal(err)
	}
	b := l.Binding()

	if b.GetLocale() != l.Locale() {
		t.Errorf("Binding.GetLocale() = %q, want %q", b.GetLocale(), l.Locale())
	}
	if err := b.SetLocale("fr"); err != nil {
		t.Fatal(err)
	}
	if l.Locale() != "fr" {
		t.Errorf("SetLocale via Binding did not update the Localizer: got %q", l.Locale())
	}
	cat := b.GetCatalog()
	if _, ok := cat["wailskit.i18n.locale_option.system"]; !ok {
		t.Errorf("Binding.GetCatalog() missing kit's own catalog entry: %#v", cat)
	}
}

// TestLocaleOptions replaces the former TestSettingsGroup — see
// settings.go's LocaleOption doc comment for why the settings.Group
// construction moved to package settings (settings.LocaleGroup) instead of
// staying a method here.
func TestLocaleOptions(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	l, err := New()
	if err != nil {
		t.Fatal(err)
	}
	options := l.LocaleOptions()

	if len(options) == 0 || options[0].Value != "system" {
		t.Fatalf("options[0] = %#v, want Value \"system\" first", options)
	}
	if options[0].Label.Key != "wailskit.i18n.locale_option.system" {
		t.Errorf("options[0].Label.Key = %q, want the system-default catalog key", options[0].Label.Key)
	}
}

func TestTWithNoArgsDoesNotMisinterpretPercent(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	l, err := New()
	if err != nil {
		t.Fatal(err)
	}
	text := T("myapp.discount", "100% off")
	if got := l.T(text); got != "100% off" {
		t.Errorf("T() = %q, want %q", got, "100% off")
	}
}
