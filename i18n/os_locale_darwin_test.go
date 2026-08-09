//go:build darwin

package i18n

import (
	"reflect"
	"testing"
)

// TestParseAppleLanguages covers the plist-array text parsing in isolation
// from exec.Command — this is the "no test ever shells out" requirement:
// readAppleLanguages (the thing that actually execs `defaults`) is never
// called by a test; only its pure parsing half is.
func TestParseAppleLanguages(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "typical multi-language output",
			in: `(
    "en-US",
    "fr-FR",
    ja
)
`,
			want: []string{"en-US", "fr-FR", "ja"},
		},
		{
			name: "single language",
			in:   "(\n    \"en-US\"\n)\n",
			want: []string{"en-US"},
		},
		{
			name: "empty",
			in:   "(\n)\n",
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseAppleLanguages(tc.in)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseAppleLanguages(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestOSLocalesIsReadAppleLanguagesByDefault pins the production wiring —
// osLocales must default to the real exec-backed implementation on darwin,
// not accidentally the no-op used on other platforms.
func TestOSLocalesIsReadAppleLanguagesByDefault(t *testing.T) {
	if reflect.ValueOf(osLocales).Pointer() != reflect.ValueOf(readAppleLanguages).Pointer() {
		t.Error("osLocales is not readAppleLanguages by default on darwin")
	}
}
