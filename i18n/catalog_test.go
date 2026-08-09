package i18n

import (
	"testing"
	"testing/fstest"
)

// TestCatalogMergeAppWins covers AD-5's "the localizer merges kit catalogs
// with app catalogs (app wins on key collision)", generalized to any two
// WithCatalog sources: later wins.
func TestCatalogMergeAppWins(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	kit := fstest.MapFS{
		"locales/en.json": &fstest.MapFile{Data: []byte(`{"k": "kit value", "kit_only": "stays"}`)},
	}
	app := fstest.MapFS{
		"locales/en.json": &fstest.MapFile{Data: []byte(`{"k": "app value"}`)},
	}

	l, err := New(WithCatalog(kit), WithCatalog(app))
	if err != nil {
		t.Fatal(err)
	}

	if got := l.T(T("k", "fallback")); got != "app value" {
		t.Errorf("T(k) = %q, want %q (later WithCatalog should win)", got, "app value")
	}
	if got := l.T(T("kit_only", "fallback")); got != "stays" {
		t.Errorf("T(kit_only) = %q, want %q (non-colliding keys survive the merge)", got, "stays")
	}
}

// TestFallbackToOther covers the missing-*key* case: a key absent from
// every catalog at every fallback tier returns Text.Other, formatted.
func TestFallbackToOther(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	text := T("myapp.totally.unknown.key", "fallback %s")
	if got := l.T(text, "text"); got != "fallback text" {
		t.Errorf("T() = %q, want %q", got, "fallback text")
	}
}

// TestCatalogValueEitherStringOrPluralObject covers the two catalog value
// shapes the format supports, and that a malformed plural object (missing
// "other", or an unknown category name) is rejected at load time rather
// than silently producing a broken catalog.
func TestCatalogValueEitherStringOrPluralObject(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	t.Run("valid mix", func(t *testing.T) {
		fsys := fstest.MapFS{
			"locales/en.json": &fstest.MapFile{Data: []byte(`{
				"plain":  "hello",
				"plural": {"one": "%d item", "other": "%d items"}
			}`)},
		}
		l, err := New(WithCatalog(fsys))
		if err != nil {
			t.Fatal(err)
		}
		if got := l.T(T("plain", "")); got != "hello" {
			t.Errorf("T(plain) = %q", got)
		}
		if got := l.TN(T("plural", "%d items"), 1); got != "1 item" {
			t.Errorf("TN(plural, 1) = %q", got)
		}
	})

	t.Run("plural object missing other", func(t *testing.T) {
		fsys := fstest.MapFS{
			"locales/en.json": &fstest.MapFile{Data: []byte(`{"bad": {"one": "%d item"}}`)},
		}
		if _, err := New(WithCatalog(fsys)); err == nil {
			t.Error("New() with a plural object missing \"other\" should fail")
		}
	})

	t.Run("unknown plural category", func(t *testing.T) {
		fsys := fstest.MapFS{
			"locales/en.json": &fstest.MapFile{Data: []byte(`{"bad": {"other": "x", "dozen": "y"}}`)},
		}
		if _, err := New(WithCatalog(fsys)); err == nil {
			t.Error("New() with an unknown plural category should fail")
		}
	})
}

// TestCatalogHydration covers Localizer.Catalog's shape: plain strings stay
// plain, plural entries come back as map[string]string keyed by CLDR
// category so the frontend can resolve them with Intl.PluralRules, and the
// current locale overlays but does not discard the default's entries.
func TestCatalogHydration(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	en := fstest.MapFS{
		"locales/en.json": &fstest.MapFile{Data: []byte(`{
			"greeting": "Hello",
			"items":    {"one": "%d item", "other": "%d items"}
		}`)},
	}
	fr := fstest.MapFS{
		"locales/fr.json": &fstest.MapFile{Data: []byte(`{"greeting": "Bonjour"}`)},
	}

	l, err := New(WithCatalog(en), WithCatalog(fr), WithLocale("fr"))
	if err != nil {
		t.Fatal(err)
	}

	cat := l.Catalog()

	if got, ok := cat["greeting"].(string); !ok || got != "Bonjour" {
		t.Errorf("Catalog()[greeting] = %#v, want the fr override %q", cat["greeting"], "Bonjour")
	}
	forms, ok := cat["items"].(map[string]string)
	if !ok {
		t.Fatalf("Catalog()[items] = %#v, want map[string]string (from the en fallback)", cat["items"])
	}
	if forms["one"] != "%d item" || forms["other"] != "%d items" {
		t.Errorf("Catalog()[items] forms = %#v", forms)
	}
}
