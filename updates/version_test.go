package updates

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    Version
		wantErr bool
	}{
		{"v1.2.3", Version{1, 2, 3, "", "v1.2.3"}, false},
		{"1.2.3", Version{1, 2, 3, "", "1.2.3"}, false},
		{"v0.0.1", Version{0, 0, 1, "", "v0.0.1"}, false},
		{"v1.2.3-beta.1", Version{1, 2, 3, "beta.1", "v1.2.3-beta.1"}, false},
		{"v1.0.0-alpha", Version{1, 0, 0, "alpha", "v1.0.0-alpha"}, false},
		{"v1.0.0+build123", Version{1, 0, 0, "", "v1.0.0+build123"}, false},
		{"", Version{}, true},
		{"v", Version{}, true},
		{"v1.2", Version{}, true},
		{"v1.2.x", Version{}, true},
		{"not-a-version", Version{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if got.Major != tt.want.Major || got.Minor != tt.want.Minor || got.Patch != tt.want.Patch || got.Prerelease != tt.want.Prerelease {
				t.Errorf("ParseVersion(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"v2.0.0", "v1.0.0", 1},
		{"v1.0.0", "v2.0.0", -1},
		{"v1.1.0", "v1.0.0", 1},
		{"v1.0.1", "v1.0.0", 1},
		// Prerelease precedence
		{"v1.0.0", "v1.0.0-alpha", 1},       // stable > prerelease
		{"v1.0.0-alpha", "v1.0.0", -1},       // prerelease < stable
		{"v1.0.0-beta", "v1.0.0-alpha", 1},   // beta > alpha
		{"v1.0.0-alpha.2", "v1.0.0-alpha.1", 1},
		{"v1.0.0-alpha.1", "v1.0.0-alpha.1", 0},
		// Numeric < alphanumeric in prerelease
		{"v1.0.0-1", "v1.0.0-alpha", -1},
		// More fields > fewer fields when equal prefix
		{"v1.0.0-alpha.1", "v1.0.0-alpha", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			a, _ := ParseVersion(tt.a)
			b, _ := ParseVersion(tt.b)
			got := a.Compare(b)
			if got != tt.want {
				t.Errorf("Compare(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestVersionNewerThan(t *testing.T) {
	a, _ := ParseVersion("v1.1.0")
	b, _ := ParseVersion("v1.0.0")

	if !a.NewerThan(b) {
		t.Error("v1.1.0 should be newer than v1.0.0")
	}
	if b.NewerThan(a) {
		t.Error("v1.0.0 should not be newer than v1.1.0")
	}
	if a.NewerThan(a) {
		t.Error("v1.1.0 should not be newer than itself")
	}
}

func TestVersionString(t *testing.T) {
	v, _ := ParseVersion("1.2.3")
	if s := v.String(); s != "v1.2.3" {
		t.Errorf("got %q, want %q", s, "v1.2.3")
	}

	v, _ = ParseVersion("v1.0.0-beta.1")
	if s := v.String(); s != "v1.0.0-beta.1" {
		t.Errorf("got %q, want %q", s, "v1.0.0-beta.1")
	}
}
