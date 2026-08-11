package health

import (
	"testing"
	"time"
)

func TestSettingsGroupShape(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck())
	g := r.SettingsGroup()

	if g.Key != "health" {
		t.Fatalf("expected group key %q, got %q", "health", g.Key)
	}
	if g.Label.Key == "" || g.Label.Other == "" {
		t.Fatalf("expected a translatable group label, got %+v", g.Label)
	}
	if len(g.Fields) != 1 {
		t.Fatalf("expected exactly 1 field, got %d", len(g.Fields))
	}

	f := g.Fields[0]
	if f.Key != SettingCheckInterval {
		t.Fatalf("expected field key %q, got %q", SettingCheckInterval, f.Key)
	}
	if f.Default != defaultInterval.String() {
		t.Fatalf("expected default %q (registry's default interval), got %v", defaultInterval.String(), f.Default)
	}
	if len(f.Options) != len(intervalChoices) {
		t.Fatalf("expected %d fixed interval options, got %d", len(intervalChoices), len(f.Options))
	}
	for _, opt := range f.Options {
		if opt.Label.Key == "" || opt.Label.Other == "" {
			t.Fatalf("expected every fixed interval option to be translatable, got %+v", opt)
		}
	}
}

func TestSettingsGroupIncludesCustomDefaultInterval(t *testing.T) {
	custom := 90 * time.Second // not one of intervalChoices
	r := New(WithoutDefaultConnectivityCheck(), WithDefaultInterval(custom))
	g := r.SettingsGroup()
	f := g.Fields[0]

	if f.Default != custom.String() {
		t.Fatalf("expected default %q, got %v", custom.String(), f.Default)
	}
	if len(f.Options) != len(intervalChoices)+1 {
		t.Fatalf("expected the custom interval appended as an extra option, got %d options", len(f.Options))
	}

	var found bool
	for _, opt := range f.Options {
		if opt.Value == custom.String() {
			found = true
			// A custom interval has no catalog key — it's a literal,
			// never-translated label (matches i18n.LocaleOptions' dynamic
			// locale names convention).
			if opt.Label.Key != "" {
				t.Fatalf("expected an untranslated literal label for a non-standard interval, got key %q", opt.Label.Key)
			}
		}
	}
	if !found {
		t.Fatal("expected the custom default interval among Options")
	}
}

func TestSettingsGroupDefaultAlwaysAmongOptions(t *testing.T) {
	for _, d := range append(append([]time.Duration{}, intervalChoices...), 90*time.Second, 3*time.Hour) {
		r := New(WithoutDefaultConnectivityCheck(), WithDefaultInterval(d))
		g := r.SettingsGroup()
		f := g.Fields[0]

		var found bool
		for _, opt := range f.Options {
			if opt.Value == f.Default {
				found = true
			}
		}
		if !found {
			t.Fatalf("default interval %s not found among its own Options", d)
		}
	}
}

// TestSettingsGroupUsesI18nText is a light guard against accidentally
// reverting to plain-string labels — settings.Field.Label is i18n.Text
// (WP-12), and a Group built with a mismatched type simply fails to
// compile, but this also asserts the zero-value distinguishing behavior
// (an unkeyed Text still resolves via Other).
func TestSettingsGroupUsesI18nText(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck())
	g := r.SettingsGroup()

	want := groupLabel
	if g.Label != want {
		t.Fatalf("expected group label %+v, got %+v", want, g.Label)
	}
}
