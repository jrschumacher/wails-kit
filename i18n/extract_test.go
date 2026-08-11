package i18n

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// i18nTCallPattern matches literal `i18n.T("key", ...)` call sites — the
// form every consumer outside this package uses (this package's own
// internal T(...) calls are exempt; its catalog is hand-verified in the
// same PR, per AD-6's "update the agent file in the same PR" rule).
//
// The key argument is matched as either a double-quoted interpreted string
// literal (group 1) or a backtick-delimited raw string literal (group 2) —
// both are valid Go for a string constant. Before this, only the
// double-quoted form was matched: a call site written with a raw string
// literal (which, being "raw", can itself contain a literal newline —
// hence "multi-line") produced zero regex matches and was entirely
// invisible to TestCatalogKeysExist, rather than being checked and
// reported missing. extractKey picks whichever group actually matched.
var i18nTCallPattern = regexp.MustCompile("i18n\\.T\\(\\s*(?:\"((?:[^\"\\\\]|\\\\.)*)\"|`([^`]*)`)")

// extractKey returns the key captured by i18nTCallPattern's match m: group 1
// for a double-quoted literal, group 2 for a backtick raw string literal —
// exactly one of the two is ever populated for a given match.
func extractKey(m [][]byte) string {
	if len(m) > 1 && len(m[1]) > 0 {
		return string(m[1])
	}
	if len(m) > 2 {
		return string(m[2])
	}
	return ""
}

// TestCatalogKeysExist is the extraction lint referenced by AD-5/WP-10: it
// walks the repository for i18n.T(...) call sites whose key is prefixed
// "wailskit." and verifies each such key exists in the locales/en.json of
// the package directory that declares it. It keeps kit catalogs honest — a
// call site added without its catalog entry is a build-time-invisible bug
// otherwise. WP-90 wires this into repo-wide CI.
//
// As of WP-10, no other kit package calls i18n.T yet (that lands in
// WP-11/12/etc.), so this test currently has nothing to check outside
// examples/i18n — but it must still be correct and repo-wide, since every
// later WP depends on it catching their mistakes.
func TestCatalogKeysExist(t *testing.T) {
	root := repoRoot(t)
	enCache := map[string]map[string]bool{}

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Errata (v2-roadmap.md §5): frontend/*/node_modules contain
			// real but vendored Go packages (flatted/golang/pkg/flatted) —
			// exclude node_modules or this scans JS-adjacent Go source.
			// Also skip VCS/tooling directories; none contain source we
			// author i18n.T calls in.
			switch d.Name() {
			case "node_modules", ".git", ".conductor", ".claude":
				return fs.SkipDir
			}
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}

		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}

		for _, m := range i18nTCallPattern.FindAllSubmatch(data, -1) {
			key := extractKey(m)
			if !strings.HasPrefix(key, "wailskit.") {
				continue
			}
			pkgDir := filepath.Dir(p)
			keys, ok := enCache[pkgDir]
			if !ok {
				keys = loadEnKeys(pkgDir)
				enCache[pkgDir] = keys
			}
			if !keys[key] {
				t.Errorf("%s: i18n.T key %q not found in %s",
					p, key, filepath.Join(pkgDir, "locales", "en.json"))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
}

// TestI18nTCallPatternMatchesBacktickLiteral pins the extraction-lint gap
// noted alongside H2/M5/M6: i18nTCallPattern used to only match a
// double-quoted key literal. A call site written with a backtick raw string
// literal instead — valid Go, and (being "raw") able to itself span
// multiple lines — produced zero matches: not a wrong key, not a reported
// miss, just silently invisible to TestCatalogKeysExist. This test drives
// the regex directly (rather than depending on a real, checked-in call site
// existing somewhere in the repo) so it fails before the fix and passes
// after, regardless of what any other package happens to contain.
//
// The fixture keys below are deliberately namespaced "myapp." rather than
// "wailskit." — TestCatalogKeysExist scans this file's own source bytes too
// (it walks the whole repo, not just non-test files), and a "wailskit."
// literal here would register as a real call site needing a catalog entry
// in locales/en.json, which is not what this test is about.
func TestI18nTCallPatternMatchesBacktickLiteral(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "double-quoted literal (baseline)",
			src:  `var X = i18n.T("myapp.example.quoted", "fallback")`,
			want: "myapp.example.quoted",
		},
		{
			name: "backtick literal, single line",
			src:  "var X = i18n.T(`myapp.example.backtick`, \"fallback\")",
			want: "myapp.example.backtick",
		},
		{
			name: "backtick literal spanning multiple lines",
			src:  "var X = i18n.T(`myapp.example\n.multiline`, \"fallback\")",
			want: "myapp.example\n.multiline",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches := i18nTCallPattern.FindAllSubmatch([]byte(tc.src), -1)
			if len(matches) != 1 {
				t.Fatalf("FindAllSubmatch found %d matches, want 1 (call site invisible to the extraction lint)", len(matches))
			}
			if got := extractKey(matches[0]); got != tc.want {
				t.Errorf("extractKey() = %q, want %q", got, tc.want)
			}
		})
	}
}

// loadEnKeys reads pkgDir/locales/en.json's top-level keys. A missing file
// yields an empty set, which is itself the failure signal for any call site
// found in that package (no catalog at all for a package declaring
// wailskit.* keys).
func loadEnKeys(pkgDir string) map[string]bool {
	data, err := os.ReadFile(filepath.Join(pkgDir, "locales", "en.json"))
	if err != nil {
		return map[string]bool{}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]bool{}
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys
}

// repoRoot finds the repository root by locating this source file (this
// test file lives at <root>/i18n/extract_test.go) rather than assuming the
// working directory — `go test` invokes tests with the package directory as
// cwd, but callers of `go test ./...` from elsewhere shouldn't break this.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("i18n: runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(file))
}
