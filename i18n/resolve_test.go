package i18n

import (
	"testing"
	"testing/fstest"
)

// enCatalogFS returns a WithCatalog source with a single locales/en.json
// containing key -> value, for tests that don't care about catalog content
// beyond having *something* the matcher can consider supported.
func enCatalogFS(t *testing.T, locale, key, value string) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"locales/" + locale + ".json": &fstest.MapFile{
			Data: []byte(`{"` + key + `":"` + value + `"}`),
		},
	}
}

// TestLocaleResolutionOrder exercises OQ-5's decided precedence, one tier
// at a time: explicit > settings > env > OS source > default.
func TestLocaleResolutionOrder(t *testing.T) {
	fr := enCatalogFS(t, "fr", "k", "bonjour")
	de := enCatalogFS(t, "de", "k", "hallo")
	ja := enCatalogFS(t, "ja", "k", "konnichiwa")
	pl := enCatalogFS(t, "pl", "k", "czesc")

	t.Run("default wins with nothing else set", func(t *testing.T) {
		clearLocaleEnv(t)
		withOSLocales(t, nil)
		l, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if got := l.Locale(); got != "en" {
			t.Errorf("Locale() = %q, want %q", got, "en")
		}
	})

	t.Run("OS source wins over default", func(t *testing.T) {
		clearLocaleEnv(t)
		withOSLocales(t, []string{"ja"})
		l, err := New(WithCatalog(ja))
		if err != nil {
			t.Fatal(err)
		}
		if got := l.Locale(); got != "ja" {
			t.Errorf("Locale() = %q, want %q", got, "ja")
		}
	})

	t.Run("env wins over OS source", func(t *testing.T) {
		clearLocaleEnv(t)
		withOSLocales(t, []string{"ja"})
		t.Setenv("LANG", "de_DE.UTF-8")
		l, err := New(WithCatalog(ja), WithCatalog(de))
		if err != nil {
			t.Fatal(err)
		}
		// The env tier is accepted as soon as it parses (see resolve.go's
		// doc comment) — it is not matched/canonicalized down to a
		// supported catalog tag, so region info survives: "de_DE.UTF-8"
		// resolves Locale() to "de-DE", not "de". Catalog lookups still
		// fall back gracefully to "de" (TestMissingLocaleFallsBackGracefully
		// covers that path directly).
		if got := l.Locale(); got != "de-DE" {
			t.Errorf("Locale() = %q, want %q", got, "de-DE")
		}
	})

	t.Run("LC_ALL wins over LANG", func(t *testing.T) {
		clearLocaleEnv(t)
		withOSLocales(t, nil)
		t.Setenv("LANG", "de_DE.UTF-8")
		t.Setenv("LC_ALL", "fr_FR.UTF-8")
		l, err := New(WithCatalog(de), WithCatalog(fr))
		if err != nil {
			t.Fatal(err)
		}
		if got := l.Locale(); got != "fr-FR" {
			t.Errorf("Locale() = %q, want %q", got, "fr-FR")
		}
	})

	t.Run("settings wins over env", func(t *testing.T) {
		clearLocaleEnv(t)
		withOSLocales(t, nil)
		t.Setenv("LC_ALL", "fr_FR.UTF-8")
		src := fakeSettings{SettingLocale: "pl"}
		l, err := New(WithCatalog(fr), WithCatalog(pl), WithSettings(src))
		if err != nil {
			t.Fatal(err)
		}
		if got := l.Locale(); got != "pl" {
			t.Errorf("Locale() = %q, want %q", got, "pl")
		}
	})

	t.Run(`settings "system" defers to lower tiers`, func(t *testing.T) {
		clearLocaleEnv(t)
		withOSLocales(t, nil)
		t.Setenv("LC_ALL", "fr_FR.UTF-8")
		src := fakeSettings{SettingLocale: "system"}
		l, err := New(WithCatalog(fr), WithSettings(src))
		if err != nil {
			t.Fatal(err)
		}
		if got := l.Locale(); got != "fr-FR" {
			t.Errorf("Locale() = %q, want %q", got, "fr-FR")
		}
	})

	t.Run("explicit WithLocale wins over everything", func(t *testing.T) {
		clearLocaleEnv(t)
		withOSLocales(t, []string{"ja"})
		t.Setenv("LC_ALL", "fr_FR.UTF-8")
		src := fakeSettings{SettingLocale: "pl"}
		l, err := New(
			WithCatalog(fr), WithCatalog(pl), WithCatalog(de),
			WithSettings(src),
			WithLocale("de"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if got := l.Locale(); got != "de" {
			t.Errorf("Locale() = %q, want %q", got, "de")
		}
	})
}

// TestOSSourceSubstituted proves osLocales is a substitutable func var — no
// test in this package (this one included) ever shells out.
func TestOSSourceSubstituted(t *testing.T) {
	clearLocaleEnv(t)
	calls := 0
	withOSLocalesFunc(t, func() []string {
		calls++
		return []string{"fr"}
	})

	fr := enCatalogFS(t, "fr", "k", "bonjour")
	l, err := New(WithCatalog(fr))
	if err != nil {
		t.Fatal(err)
	}
	if got := l.Locale(); got != "fr" {
		t.Errorf("Locale() = %q, want %q", got, "fr")
	}
	if calls != 1 {
		t.Errorf("osLocales called %d times, want exactly 1 (resolved once at construction)", calls)
	}
}

// TestMissingLocaleFallsBackGracefully covers the "missing-locale fallback"
// case explicitly (distinct from a missing *key*, see TestFallbackToOther):
// a resolved locale with no catalog of its own must still find strings via
// an ancestor or the configured default, and Locale() must still report
// what was actually requested.
func TestMissingLocaleFallsBackGracefully(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)
	l, err := New(WithLocale("de")) // no "de" catalog anywhere; only kit "en"
	if err != nil {
		t.Fatal(err)
	}
	if got := l.Locale(); got != "de" {
		t.Errorf("Locale() = %q, want %q (should report what was requested)", got, "de")
	}
	got := l.T(systemLocaleLabel)
	if got != systemLocaleLabel.Other {
		t.Errorf("T() = %q, want the English kit fallback %q", got, systemLocaleLabel.Other)
	}
}

func TestParseEnvLocale(t *testing.T) {
	cases := []struct {
		in      string
		wantTag string
		wantOK  bool
	}{
		{"en_US.UTF-8", "en-US", true},
		{"fr_CA.UTF-8@euro", "fr-CA", true},
		{"ja", "ja", true},
		{"C", "", false},
		{"POSIX", "", false},
		{"", "", false},
		{"not a locale!!", "", false},
	}
	for _, tc := range cases {
		tag, ok := parseEnvLocale(tc.in)
		if ok != tc.wantOK {
			t.Errorf("parseEnvLocale(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
			continue
		}
		if ok && tag.String() != tc.wantTag {
			t.Errorf("parseEnvLocale(%q) = %q, want %q", tc.in, tag.String(), tc.wantTag)
		}
	}
}

// fakeSettings is a minimal SettingsSource test double.
type fakeSettings map[string]any

func (f fakeSettings) GetValues() (map[string]any, error) {
	return f, nil
}

// withOSLocales substitutes osLocales for the duration of the test and
// restores the original afterward.
func withOSLocales(t *testing.T, tags []string) {
	t.Helper()
	withOSLocalesFunc(t, func() []string { return tags })
}

func withOSLocalesFunc(t *testing.T, fn func() []string) {
	t.Helper()
	orig := osLocales
	osLocales = fn
	t.Cleanup(func() { osLocales = orig })
}

// clearLocaleEnv blanks the three env vars resolveInitial checks so tests
// get deterministic behavior regardless of the host running them (a
// developer machine or CI runner may well have LANG set).
func clearLocaleEnv(t *testing.T) {
	t.Helper()
	for _, name := range envLocaleVars {
		t.Setenv(name, "")
	}
}
