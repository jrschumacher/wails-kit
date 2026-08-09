package i18n

import (
	"testing"
	"testing/fstest"
)

// TestPluralCLDR exercises real CLDR plural rules via
// golang.org/x/text/feature/plural — not hand-rolled logic — for a language
// English can't test: Polish has four cardinal categories (one/few/many/
// other), so this also proves "few" and "many" are reachable and distinct,
// which an English-only fixture never could.
func TestPluralCLDR(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	pl := fstest.MapFS{
		"locales/pl.json": &fstest.MapFile{Data: []byte(`{
			"items": {
				"one":   "%d plik",
				"few":   "%d pliki",
				"many":  "%d plikow",
				"other": "%d pliku"
			}
		}`)},
	}

	l, err := New(WithCatalog(pl), WithLocale("pl"))
	if err != nil {
		t.Fatal(err)
	}

	text := T("items", "%d files")

	// Polish cardinal rule (CLDR 47): one -> n=1; few -> n%10 in 2..4 &&
	// n%100 not in 12..14; many -> everything else (incl. n=5..21 and n%10
	// in 0,1,5..9 or n%100 in 12..14); other -> only for non-integer counts,
	// unreachable here.
	cases := []struct {
		n    int
		want string
	}{
		{1, "1 plik"},     // one
		{2, "2 pliki"},    // few
		{3, "3 pliki"},    // few
		{4, "4 pliki"},    // few
		{5, "5 plikow"},   // many
		{12, "12 plikow"}, // many (12-14 excluded from "few")
		{22, "22 pliki"},  // few (n%10==2, n%100!=12)
		{0, "0 plikow"},   // many
	}
	for _, tc := range cases {
		if got := l.TN(text, tc.n); got != tc.want {
			t.Errorf("TN(items, %d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// TestPluralFallsBackToOtherWhenFormMissing covers a plural entry that
// doesn't define every category the resolved locale's rule can select —
// TN must fall back to "other" rather than producing an empty/garbled
// string.
func TestPluralFallsBackToOtherWhenFormMissing(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	pl := fstest.MapFS{
		"locales/pl.json": &fstest.MapFile{Data: []byte(`{
			"partial": {"one": "%d rzecz", "other": "%d rzeczy"}
		}`)},
	}
	l, err := New(WithCatalog(pl), WithLocale("pl"))
	if err != nil {
		t.Fatal(err)
	}

	text := T("partial", "%d things")
	// n=3 selects Polish "few", which this entry doesn't define.
	if got := l.TN(text, 3); got != "3 rzeczy" {
		t.Errorf("TN(partial, 3) = %q, want %q (other fallback)", got, "3 rzeczy")
	}
}

// TestPluralNonPluralEntryUsesOther covers TN against a key whose catalog
// entry is a plain string (not a plural object) — every argument still
// formats correctly, with n first.
func TestPluralNonPluralEntryUsesOther(t *testing.T) {
	clearLocaleEnv(t)
	withOSLocales(t, nil)

	en := fstest.MapFS{
		"locales/en.json": &fstest.MapFile{Data: []byte(`{"greeting": "%d greetings for %s"}`)},
	}
	l, err := New(WithCatalog(en))
	if err != nil {
		t.Fatal(err)
	}
	text := T("greeting", "%d greetings for %s (fallback)")
	if got := l.TN(text, 2, "Ryan"); got != "2 greetings for Ryan" {
		t.Errorf("TN = %q", got)
	}
}
