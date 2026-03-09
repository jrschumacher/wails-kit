package updates

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a parsed semantic version.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Raw        string
}

// ParseVersion parses a version string like "v1.2.3" or "1.2.3-beta.1".
func ParseVersion(s string) (Version, error) {
	raw := s
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return Version{}, fmt.Errorf("empty version string")
	}

	// Split off build metadata first (ignored per semver spec).
	// This must happen before splitting prerelease so that
	// "1.0.0-beta+build" does not leak "+build" into the prerelease.
	if idx := strings.IndexByte(s, '+'); idx >= 0 {
		s = s[:idx]
	}

	// Split off prerelease
	var pre string
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		pre = s[idx+1:]
		s = s[:idx]
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version %q: expected MAJOR.MINOR.PATCH", raw)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version %q: %w", parts[0], err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version %q: %w", parts[1], err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch version %q: %w", parts[2], err)
	}

	return Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: pre,
		Raw:        raw,
	}, nil
}

// String returns the canonical "vMAJOR.MINOR.PATCH[-PRERELEASE]" form.
func (v Version) String() string {
	s := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	return s
}

// Compare returns -1 if v < other, 0 if v == other, 1 if v > other.
func (v Version) Compare(other Version) int {
	if c := cmpInt(v.Major, other.Major); c != 0 {
		return c
	}
	if c := cmpInt(v.Minor, other.Minor); c != 0 {
		return c
	}
	if c := cmpInt(v.Patch, other.Patch); c != 0 {
		return c
	}
	return comparePrerelease(v.Prerelease, other.Prerelease)
}

// NewerThan returns true if v is strictly newer than other.
func (v Version) NewerThan(other Version) bool {
	return v.Compare(other) > 0
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// comparePrerelease implements semver prerelease precedence:
// - No prerelease > any prerelease (1.0.0 > 1.0.0-alpha)
// - Dot-separated identifiers compared left to right
// - Numeric identifiers compared as integers; alphanumeric as strings
// - Numeric < alphanumeric; fewer fields < more fields (when all preceding are equal)
func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1 // stable > prerelease
	}
	if b == "" {
		return -1
	}

	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		aNum, aErr := strconv.Atoi(aParts[i])
		bNum, bErr := strconv.Atoi(bParts[i])

		switch {
		case aErr == nil && bErr == nil:
			if c := cmpInt(aNum, bNum); c != 0 {
				return c
			}
		case aErr == nil:
			return -1 // numeric < alphanumeric
		case bErr == nil:
			return 1
		default:
			if aParts[i] < bParts[i] {
				return -1
			}
			if aParts[i] > bParts[i] {
				return 1
			}
		}
	}

	return cmpInt(len(aParts), len(bParts))
}
