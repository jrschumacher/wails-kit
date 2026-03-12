package updates

import "testing"

func TestInstallMethodUpdateInstructions(t *testing.T) {
	tests := []struct {
		method InstallMethod
		app    string
		want   string
	}{
		{InstallDirect, "myapp", ""},
		{InstallHomebrew, "myapp", "This app was installed via Homebrew. Run: brew upgrade --cask myapp"},
	}

	for _, tt := range tests {
		got := tt.method.UpdateInstructions(tt.app)
		if got != tt.want {
			t.Errorf("InstallMethod(%q).UpdateInstructions(%q) = %q, want %q", tt.method, tt.app, got, tt.want)
		}
	}
}

func TestDetectInstallMethodDirect(t *testing.T) {
	// When running tests, the executable is the test binary which
	// is not inside a Homebrew Caskroom, so it should be direct.
	method := DetectInstallMethod()
	if method != InstallDirect {
		t.Errorf("expected InstallDirect, got %q", method)
	}
}
