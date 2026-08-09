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
var i18nTCallPattern = regexp.MustCompile(`i18n\.T\(\s*"((?:[^"\\]|\\.)*)"`)

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
			key := string(m[1])
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
