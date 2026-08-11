package appearance

import (
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// newTestSettings builds a settings.Service backed by a temp file — never
// the real OS-standard path (settings/AGENTS.md's landmine: a test that
// calls SetValues against the default path pollutes the developer's
// filesystem across separate `go test` runs).
func newTestSettings(t *testing.T, opts ...settings.ServiceOption) *settings.Service {
	t.Helper()
	base := []settings.ServiceOption{
		settings.WithStoragePath(filepath.Join(t.TempDir(), "settings.json")),
	}
	return settings.NewService(append(base, opts...)...)
}

// TestResolveTheme covers the pure resolution rule for all three Mode
// values against every Source state, including a nil Source — this is the
// "three states, not two" requirement plus the documented nil-source
// default.
func TestResolveTheme(t *testing.T) {
	cases := []struct {
		name   string
		mode   Mode
		source Source
		want   Theme
	}{
		{"light mode ignores nil source", ModeLight, nil, ThemeLight},
		{"dark mode ignores nil source", ModeDark, nil, ThemeDark},
		{"system mode, nil source, defaults light", ModeSystem, nil, ThemeLight},
		{"system mode, source light", ModeSystem, newFakeSource(false), ThemeLight},
		{"system mode, source dark", ModeSystem, newFakeSource(true), ThemeDark},
		{"light mode overrides dark source", ModeLight, newFakeSource(true), ThemeLight},
		{"dark mode overrides light source", ModeDark, newFakeSource(false), ThemeDark},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveTheme(tc.mode, tc.source); got != tc.want {
				t.Errorf("resolveTheme(%q, %v) = %q, want %q", tc.mode, tc.source, got, tc.want)
			}
		})
	}
}

// TestNewServiceDefaults pins the zero-option construction: ModeSystem,
// and (via an explicit WithSource(nil) so this test never depends on the
// platform default) a resolved Theme of light.
func TestNewServiceDefaults(t *testing.T) {
	s := NewService(WithSource(nil))
	if got := s.Mode(); got != ModeSystem {
		t.Errorf("Mode() = %q, want %q", got, ModeSystem)
	}
	if got := s.Resolved(); got != ThemeLight {
		t.Errorf("Resolved() = %q, want %q", got, ThemeLight)
	}
}

// TestResolveOverridesSystem is the override-precedence-over-OS test named
// in docs/v2-roadmap.md's WP-13 entry: an explicit mode wins over whatever
// the Source reports, in both directions.
func TestResolveOverridesSystem(t *testing.T) {
	src := newFakeSource(true) // OS is dark
	s := NewService(WithSource(src))

	if got := s.Resolved(); got != ThemeDark {
		t.Fatalf("precondition: Resolved() = %q, want %q (system should follow dark OS)", got, ThemeDark)
	}

	if err := s.SetMode(ModeLight); err != nil {
		t.Fatalf("SetMode(ModeLight): %v", err)
	}
	if got := s.Resolved(); got != ThemeLight {
		t.Errorf("after SetMode(ModeLight) with dark OS: Resolved() = %q, want %q", got, ThemeLight)
	}
	if got := s.Mode(); got != ModeLight {
		t.Errorf("Mode() = %q, want %q", got, ModeLight)
	}

	// Flipping the OS further must not move the resolved theme while an
	// override is in effect.
	src.flip(false)
	if got := s.Resolved(); got != ThemeLight {
		t.Errorf("after OS flip with ModeLight override: Resolved() = %q, want %q", got, ThemeLight)
	}
}

// TestSystemFlipEmits is the OS-flip-while-on-follow-OS test named in
// docs/v2-roadmap.md's WP-13 entry: flipping the Source while Mode is
// ModeSystem must resolve to the new theme and emit EventChanged exactly
// once.
func TestSystemFlipEmits(t *testing.T) {
	src := newFakeSource(false) // OS starts light
	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	s := NewService(WithSource(src), WithEmitter(emitter))
	if got := s.Resolved(); got != ThemeLight {
		t.Fatalf("precondition: Resolved() = %q, want %q", got, ThemeLight)
	}

	src.flip(true) // OS -> dark

	if got := s.Resolved(); got != ThemeDark {
		t.Errorf("after OS flip: Resolved() = %q, want %q", got, ThemeDark)
	}

	records := mem.Events()
	var changed []events.Record
	for _, r := range records {
		if r.Name == EventChanged {
			changed = append(changed, r)
		}
	}
	if len(changed) != 1 {
		t.Fatalf("got %d %s events, want exactly 1: %+v", len(changed), EventChanged, changed)
	}
	payload, ok := changed[0].Data.(ChangedPayload)
	if !ok {
		t.Fatalf("event payload type = %T, want ChangedPayload", changed[0].Data)
	}
	if payload.Mode != ModeSystem || payload.Resolved != ThemeDark {
		t.Errorf("payload = %+v, want {Mode: system, Resolved: dark}", payload)
	}
}

// TestNoEventWhenResolvedUnchanged covers two ways a change can be a no-op
// at the resolved-Theme level even though something nominally happened:
// setting the same mode again, and setting a different mode that happens
// to resolve to the same theme the OS was already reporting.
func TestNoEventWhenResolvedUnchanged(t *testing.T) {
	t.Run("SetMode with same mode", func(t *testing.T) {
		src := newFakeSource(false)
		mem := events.NewMemoryEmitter()
		s := NewService(WithSource(src), WithEmitter(events.NewEmitter(mem)))

		if err := s.SetMode(ModeSystem); err != nil {
			t.Fatalf("SetMode: %v", err)
		}
		if mem.Count() != 0 {
			t.Errorf("got %d events after a no-op SetMode, want 0: %+v", mem.Count(), mem.Events())
		}
	})

	t.Run("SetMode resolves to the same theme the OS already had", func(t *testing.T) {
		src := newFakeSource(true) // OS dark
		mem := events.NewMemoryEmitter()
		s := NewService(WithSource(src), WithEmitter(events.NewEmitter(mem)))
		if got := s.Resolved(); got != ThemeDark {
			t.Fatalf("precondition: Resolved() = %q, want dark", got)
		}

		// ModeSystem -> ModeDark: the mode changes, but the resolved
		// theme (dark) does not, because the OS was already dark.
		if err := s.SetMode(ModeDark); err != nil {
			t.Fatalf("SetMode: %v", err)
		}
		if mem.Count() != 0 {
			t.Errorf("got %d events, want 0 (resolved theme didn't change): %+v", mem.Count(), mem.Events())
		}
	})

	t.Run("OS flip while an override is in effect", func(t *testing.T) {
		src := newFakeSource(false)
		mem := events.NewMemoryEmitter()
		s := NewService(WithSource(src), WithEmitter(events.NewEmitter(mem)))
		if err := s.SetMode(ModeLight); err != nil {
			t.Fatalf("SetMode: %v", err)
		}
		mem.Clear()

		src.flip(true) // OS -> dark, but ModeLight override should absorb this
		if mem.Count() != 0 {
			t.Errorf("got %d events from an OS flip while overridden, want 0: %+v", mem.Count(), mem.Events())
		}
	})
}

// TestPersistViaSettings is the settings-persistence test named in
// docs/v2-roadmap.md's WP-13 entry: SetMode persists through the wired
// settings.Service, and Mode()/Resolved() read it back live — including
// from a second, independently-constructed Service sharing the same
// settings.Service, which is the realistic "app restart" shape.
func TestPersistViaSettings(t *testing.T) {
	svc := newTestSettings(t)

	s1 := NewService(WithSource(newFakeSource(false)), WithSettings(svc))
	if err := s1.SetMode(ModeDark); err != nil {
		t.Fatalf("SetMode: %v", err)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if got := values[SettingMode]; got != string(ModeDark) {
		t.Errorf("persisted %s = %v, want %q", SettingMode, got, ModeDark)
	}

	// A second Service, wired to the same settings.Service, must read the
	// override back live rather than defaulting to ModeSystem.
	s2 := NewService(WithSource(newFakeSource(false)), WithSettings(svc))
	if got := s2.Mode(); got != ModeDark {
		t.Errorf("s2.Mode() = %q, want %q (read back from shared settings.Service)", got, ModeDark)
	}
	if got := s2.Resolved(); got != ThemeDark {
		t.Errorf("s2.Resolved() = %q, want %q", got, ThemeDark)
	}

	// And changes made directly through settings.SetValues (the generic
	// settings-UI path, bypassing SetMode entirely) are visible to Mode()
	// on the *first* Service too, because Mode() reads live.
	if _, err := svc.SetValues(map[string]any{SettingMode: string(ModeLight)}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if got := s1.Mode(); got != ModeLight {
		t.Errorf("s1.Mode() after external SetValues = %q, want %q", got, ModeLight)
	}
}

// TestRefreshEmitsOnExternalSettingsChange demonstrates the documented
// Refresh() wiring for the composition gap TestPersistViaSettings's last
// assertion exposes: Mode() picks up an external settings write live, but
// nothing emits EventChanged for it unless something calls Refresh.
func TestRefreshEmitsOnExternalSettingsChange(t *testing.T) {
	svc := newTestSettings(t)
	mem := events.NewMemoryEmitter()
	s := NewService(WithSource(newFakeSource(false)), WithSettings(svc), WithEmitter(events.NewEmitter(mem)))

	if _, err := svc.SetValues(map[string]any{SettingMode: string(ModeDark)}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if mem.Count() != 0 {
		t.Fatalf("external SetValues alone must not emit — got %d events", mem.Count())
	}

	s.Refresh()

	if got := s.Resolved(); got != ThemeDark {
		t.Errorf("Resolved() after Refresh = %q, want %q", got, ThemeDark)
	}
	changed := mem.Events()
	if len(changed) != 1 || changed[0].Name != EventChanged {
		t.Fatalf("got events %+v, want exactly one %s", changed, EventChanged)
	}

	// A second Refresh with nothing having changed must not emit again.
	mem.Clear()
	s.Refresh()
	if mem.Count() != 0 {
		t.Errorf("Refresh with no actual change emitted %d events, want 0", mem.Count())
	}
}

// TestWithSourceSubstitutesPlatformDefault is the "substituted OS source"
// coverage required by the WP-13 spec: WithSource(fake) — not the platform
// default — is what NewService actually uses.
func TestWithSourceSubstitutesPlatformDefault(t *testing.T) {
	src := newFakeSource(true)
	s := NewService(WithSource(src))
	if got := s.Resolved(); got != ThemeDark {
		t.Errorf("Resolved() = %q, want %q (fake Source's IsDark should be authoritative)", got, ThemeDark)
	}
}

func TestSetModeRejectsInvalidMode(t *testing.T) {
	s := NewService(WithSource(nil))
	err := s.SetMode(Mode("sepia"))
	if err == nil {
		t.Fatal("SetMode(\"sepia\") returned nil error, want ErrInvalidMode")
	}
	if got := s.Mode(); got != ModeSystem {
		t.Errorf("Mode() after rejected SetMode = %q, want unchanged %q", got, ModeSystem)
	}
}

func TestClose_UnsubscribesFromSource(t *testing.T) {
	src := newFakeSource(false)
	s := NewService(WithSource(src))
	if got := src.subscriberCount(); got != 1 {
		t.Fatalf("subscriberCount() after NewService = %d, want 1", got)
	}
	s.Close()
	if got := src.subscriberCount(); got != 0 {
		t.Errorf("subscriberCount() after Close = %d, want 0", got)
	}
	if src.cancelled != 1 {
		t.Errorf("cancelled = %d, want 1", src.cancelled)
	}
}

func TestClose_NoSourceIsNoop(t *testing.T) {
	s := NewService(WithSource(nil))
	s.Close() // must not panic
}

func TestCurrentMode_FallsBackOnInvalidStoredValue(t *testing.T) {
	svc := newTestSettings(t, settings.WithGroup(settings.Group{
		Key: "appearance",
		Fields: []settings.Field{
			{Key: SettingMode, Type: settings.FieldText, Default: "not-a-real-mode"},
		},
	}))
	s := NewService(WithSource(nil), WithSettings(svc))
	if got := s.Mode(); got != ModeSystem {
		t.Errorf("Mode() with an invalid stored value = %q, want fallback %q", got, ModeSystem)
	}
}

// TestSettingsGroup_Shape checks SettingsGroup's structure independent of
// i18n resolution: settings.Field/Group.Label are i18n.Text (WP-12) — this
// package builds them, but resolving Text to a plain string is
// settings.Service.GetSchema's job, not this package's. See
// TestSettingsGroup_ResolvesThroughSettingsService for the resolved wire
// shape.
func TestSettingsGroup_Shape(t *testing.T) {
	g := SettingsGroup()
	if g.Key != "appearance" {
		t.Fatalf("group key = %q, want %q", g.Key, "appearance")
	}
	if g.Label.Other != "Appearance" {
		t.Errorf("group label.Other = %q, want %q", g.Label.Other, "Appearance")
	}
	if len(g.Fields) != 1 || g.Fields[0].Key != SettingMode {
		t.Fatalf("fields = %+v, want exactly one field keyed %q", g.Fields, SettingMode)
	}
	field := g.Fields[0]
	if field.Label.Other != "Theme" {
		t.Errorf("field label.Other = %q, want %q", field.Label.Other, "Theme")
	}
	wantValues := []string{string(ModeSystem), string(ModeLight), string(ModeDark)}
	if len(field.Options) != len(wantValues) {
		t.Fatalf("got %d options, want %d", len(field.Options), len(wantValues))
	}
	for i, want := range wantValues {
		if field.Options[i].Value != want {
			t.Errorf("option[%d].Value = %q, want %q", i, field.Options[i].Value, want)
		}
	}
}

// TestSettingsGroup_ResolvesThroughSettingsService is the real integration
// path: SettingsGroup is registered with a settings.Service, and
// GetSchema() resolves its i18n.Text labels — through the service's wired
// Localizer if there is one, otherwise through each Text's built-in
// English fallback.
func TestSettingsGroup_ResolvesThroughSettingsService(t *testing.T) {
	t.Run("without a localizer, falls back to English", func(t *testing.T) {
		svc := newTestSettings(t, settings.WithGroup(SettingsGroup()))
		schema := svc.GetSchema()
		group := findGroup(t, schema, "appearance")
		if group.Label != "Appearance" {
			t.Errorf("resolved group label = %q, want %q", group.Label, "Appearance")
		}
		if len(group.Fields) != 1 || group.Fields[0].Label != "Theme" {
			t.Errorf("resolved field = %+v, want label %q", group.Fields, "Theme")
		}
	})

	t.Run("with a localizer, resolves through the catalog", func(t *testing.T) {
		fr := fstest.MapFS{
			"locales/fr.json": &fstest.MapFile{
				Data: []byte(`{"wailskit.appearance.group.label": "Apparence"}`),
			},
		}
		loc, err := i18n.New(i18n.WithCatalog(fr))
		if err != nil {
			t.Fatalf("i18n.New: %v", err)
		}
		if err := loc.SetLocale("fr"); err != nil {
			t.Fatalf("SetLocale: %v", err)
		}

		svc := newTestSettings(t, settings.WithGroup(SettingsGroup()), settings.WithLocalizer(loc))
		schema := svc.GetSchema()
		group := findGroup(t, schema, "appearance")
		if group.Label != "Apparence" {
			t.Errorf("resolved group label = %q, want translated %q", group.Label, "Apparence")
		}
	})
}

func findGroup(t *testing.T, schema settings.ResolvedSchema, key string) settings.ResolvedGroup {
	t.Helper()
	for _, g := range schema.Groups {
		if g.Key == key {
			return g
		}
	}
	t.Fatalf("no group %q in schema: %+v", key, schema)
	return settings.ResolvedGroup{}
}

func TestBinding_DelegatesToService(t *testing.T) {
	svc := newTestSettings(t)
	s := NewService(WithSource(newFakeSource(false)), WithSettings(svc))
	b := s.Binding()

	if got := b.GetMode(); got != ModeSystem {
		t.Errorf("GetMode() = %q, want %q", got, ModeSystem)
	}
	if err := b.SetMode(ModeDark); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if got := b.GetResolved(); got != ThemeDark {
		t.Errorf("GetResolved() = %q, want %q", got, ThemeDark)
	}
	if got := s.Mode(); got != ModeDark {
		t.Errorf("underlying Service.Mode() = %q, want %q (Binding must delegate, not shadow)", got, ModeDark)
	}
}

// TestConcurrentAccess exercises SetMode, Mode, Resolved, and a concurrent
// OS flip together under -race, mirroring the concurrency guarantees every
// other kit service documents.
func TestConcurrentAccess(t *testing.T) {
	src := newFakeSource(false)
	svc := newTestSettings(t)
	s := NewService(WithSource(src), WithSettings(svc), WithEmitter(events.NewEmitter(events.NewMemoryEmitter())))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			_ = s.Mode()
			_ = s.Resolved()
		}
	}()

	for i := 0; i < 20; i++ {
		mode := ModeLight
		if i%2 == 0 {
			mode = ModeDark
		}
		_ = s.SetMode(mode)
		src.flip(i%3 == 0)
	}
	<-done
	s.Close()
}
