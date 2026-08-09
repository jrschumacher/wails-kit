package kit

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNoWailsImport is defense-in-depth on top of the AD-4 CI check
// (.github/scripts/check-wails-imports.sh, docs/v2-roadmap.md): kit is
// deliberately absent from the Wails-import allowlist because it must run
// unmodified in Prune's CLI and TUI processes, which never construct an
// application.App. That property is the entire reason kit/wailsbridge is a
// separate package (AD-1) rather than something kit does itself — see
// AGENTS.md, "kit is Wails-free". This test makes the property visible
// directly from `go test ./kit/...`, not only from a separate CI step, and
// checks the kit package specifically — not kit/wailsbridge, which will
// legitimately import wails/v3 once WP-31 adds it.
func TestNoWailsImport(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	dir := filepath.Dir(thisFile)

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps .: %v\n%s", err, out)
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "github.com/wailsapp/wails/") || line == "github.com/wailsapp/wails" {
			t.Errorf("kit depends on Wails package %q — kit must stay Wails-free (AD-4); Wails glue belongs in kit/wailsbridge", line)
		}
	}
}
