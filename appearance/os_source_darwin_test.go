//go:build darwin

package appearance

import (
	"errors"
	"reflect"
	"testing"
)

// TestReadAppleInterfaceStyleIsDefaultByDefault pins the production
// wiring — readAppleInterfaceStyle must default to the real exec-backed
// implementation, not accidentally something else — without ever calling
// it (reflect.Value.Pointer comparison only, no exec). Mirrors
// i18n/os_locale_darwin_test.go's TestOSLocalesIsReadAppleLanguagesByDefault.
func TestReadAppleInterfaceStyleIsDefaultByDefault(t *testing.T) {
	if reflect.ValueOf(readAppleInterfaceStyle).Pointer() != reflect.ValueOf(defaultReadAppleInterfaceStyle).Pointer() {
		t.Error("readAppleInterfaceStyle is not defaultReadAppleInterfaceStyle by default")
	}
}

// TestNewDefaultSource_ParsesDarkAndLight substitutes readAppleInterfaceStyle
// (this package's "no test ever shells out" seam — see its doc comment) to
// exercise newDefaultSource's Dark/Light/error-handling logic without
// executing `defaults`.
func TestNewDefaultSource_ParsesDarkAndLight(t *testing.T) {
	orig := readAppleInterfaceStyle
	t.Cleanup(func() { readAppleInterfaceStyle = orig })

	cases := []struct {
		name     string
		out      string
		err      error
		wantDark bool
	}{
		{"dark", "Dark", nil, true},
		{"light: command errors (no such key)", "", errors.New("exit status 1"), false},
		{"unexpected output treated as light", "Light", nil, false},
		{"empty output treated as light", "", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			readAppleInterfaceStyle = func() (string, error) { return tc.out, tc.err }
			src := newDefaultSource()
			if got := src.IsDark(); got != tc.wantDark {
				t.Errorf("IsDark() = %v, want %v", got, tc.wantDark)
			}
		})
	}
}

// TestDarwinSource_CachesAtConstruction pins the documented behavior: the
// darwin default Source resolves once, at construction, and never re-execs
// — a later change to what readAppleInterfaceStyle would return must not
// affect an already-constructed Source's IsDark().
func TestDarwinSource_CachesAtConstruction(t *testing.T) {
	orig := readAppleInterfaceStyle
	t.Cleanup(func() { readAppleInterfaceStyle = orig })

	readAppleInterfaceStyle = func() (string, error) { return "Dark", nil }
	src := newDefaultSource()
	if !src.IsDark() {
		t.Fatal("precondition: IsDark() = false, want true")
	}

	readAppleInterfaceStyle = func() (string, error) { return "", errors.New("boom") }
	if !src.IsDark() {
		t.Error("IsDark() changed after construction — the darwin default source must cache, not re-exec")
	}
}

// TestDarwinSource_SubscribeIsInert pins the documented "no live signal
// without Wails" behavior: Subscribe never calls fn, and its cancel is a
// harmless no-op.
func TestDarwinSource_SubscribeIsInert(t *testing.T) {
	src := &darwinSource{dark: false}
	called := false
	cancel := src.Subscribe(func(bool) { called = true })
	cancel() // must not panic
	if called {
		t.Error("the darwin default Source's Subscribe called fn, but it has no live signal to report")
	}
}
