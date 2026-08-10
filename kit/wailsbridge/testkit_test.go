package wailsbridge

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/health"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/kit"
)

// testDirs roots every path a test Kit touches under t.TempDir() — see
// kit/kit_test.go's identical helper; duplicated here because it's
// unexported in package kit.
func testDirs(t *testing.T) *appdirs.Dirs {
	t.Helper()
	root := t.TempDir()
	return appdirs.New("wailsbridge-test-app",
		appdirs.WithConfigDir(filepath.Join(root, "config")),
		appdirs.WithDataDir(filepath.Join(root, "data")),
		appdirs.WithCacheDir(filepath.Join(root, "cache")),
		appdirs.WithLogDir(filepath.Join(root, "log")),
		appdirs.WithTempDir(filepath.Join(root, "temp")),
	)
}

// newTestKit builds a *kit.Kit hermetically: a temp-dir Dirs, a
// non-OS keyring, and the default health connectivity check disabled — no
// test in this package may touch the real OS keychain or the network. See
// kit/AGENTS.md's identical baseOpts rationale.
func newTestKit(t *testing.T, opts ...kit.Option) *kit.Kit {
	t.Helper()
	all := append([]kit.Option{
		kit.WithDirs(testDirs(t)),
		kit.WithKeyring(keyring.NewMemoryStore()),
		kit.WithHealthOptions(health.WithoutDefaultConnectivityCheck()),
	}, opts...)
	k, err := kit.New(kit.AppInfo{Name: "wailsbridge-test-app", Version: "1.0.0"}, all...)
	if err != nil {
		t.Fatalf("kit.New: %v", err)
	}
	return k
}

// testApp returns the process-wide *application.App, constructing it once.
// wails/v3's application.New is a singleton (globalApplication) — every
// call after the first returns the same instance regardless of the
// Options passed — so every test in this package must share one app,
// exactly like shortcuts/apply_test.go's identical testApp helper (which
// this package cannot import, being a different package). Run() is never
// called, so this never needs a display: application.New's init() wires
// every Manager (Event, Menu, Env, ...) without touching the platform impl.
var (
	testAppOnce sync.Once
	testAppVal  *application.App
)

func testApp(t *testing.T) *application.App {
	t.Helper()
	testAppOnce.Do(func() {
		testAppVal = application.New(application.Options{Name: "wailsbridge-test-app"})
	})
	return testAppVal
}
