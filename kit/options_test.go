package kit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jrschumacher/wails-kit/v2/settings"
	"github.com/jrschumacher/wails-kit/v2/updates"
)

// esCatalog is a minimal app catalog containing an "es" locale, so
// "es" is a valid settings.LocaleGroup select option in the tests below —
// v2 ships English only in the kit's own embedded catalogs (AD-5), so
// without an app-supplied catalog no non-"system" value would validate.
var esCatalog = fstest.MapFS{
	"locales/es.json": {Data: []byte(`{"wailskit.kit.test.greeting":"hola"}`)},
}

// TestWithSettingsStoragePath covers the workspace-local settings case
// (docs/v2-roadmap.md §1, Prune): settings persist at the given path
// instead of the OS config directory, and i18n's peek (which must read the
// same file — see AGENTS.md) sees a locale set through it.
func TestWithSettingsStoragePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace-settings.json")

	k := newTestKit(t, WithSettingsStoragePath(path), WithLocales(esCatalog))

	if _, err := os.Stat(path); err == nil {
		t.Fatalf("settings file exists before any SetValues call")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}

	if verrs, err := k.Settings.SetValues(map[string]any{"i18n.locale": "es"}); err != nil || len(verrs) > 0 {
		t.Fatalf("SetValues: err=%v verrs=%v", err, verrs)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings file was not created at the configured path: %v", err)
	}

	// A second Kit built against the same path (fresh i18n peek) should see
	// the persisted override immediately, before any SetLocale call.
	k2 := newTestKit(t, WithSettingsStoragePath(path), WithLocales(esCatalog))
	if got := k2.I18n.Locale(); got != "es" {
		t.Errorf("second Kit's initial locale = %q, want %q (persisted via the first Kit's settings)", got, "es")
	}
}

// TestWithSettingsGroupAppearsAfterKitGroups covers the documented
// ordering: app-supplied groups (WithSettingsGroup) come after every
// enabled kit-internal group.
func TestWithSettingsGroupAppearsAfterKitGroups(t *testing.T) {
	appGroup := settings.Group{Key: "app", Fields: []settings.Field{{Key: "app.setting", Type: settings.FieldText}}}
	k := newTestKit(t, WithSettingsGroup(appGroup))

	groups := k.Settings.GetSchema().Groups
	if len(groups) == 0 {
		t.Fatal("no settings groups at all")
	}
	last := groups[len(groups)-1]
	if last.Key != "app" {
		t.Errorf("last settings group = %q, want the app-supplied group (%q) last", last.Key, "app")
	}
}

// TestWithLocaleForcesInitialLocale covers the explicit-override tier —
// e.g. a CLI's --locale flag — taking precedence over the (absent, in this
// test) persisted setting.
func TestWithLocaleForcesInitialLocale(t *testing.T) {
	k := newTestKit(t, WithLocale("fr"))
	if got := k.I18n.Locale(); got != "fr" {
		t.Errorf("Locale() = %q, want %q", got, "fr")
	}
}

// TestUpdatesTickerChecksOnStartup covers the "updates scheduler" lifecycle
// service end-to-end against a fake GitHub API: Start triggers at least one
// CheckForUpdate, observable via the updates:available event.
func TestUpdatesTickerChecksOnStartup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","name":"v9.9.9","assets":[]}`))
	}))
	defer srv.Close()

	k := newTestKit(t,
		WithGitHubRepo("acme", "widget"),
		WithUpdatesOptions(updates.WithGitHubAPIURL(srv.URL)),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := k.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = k.Close() }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if k.Updates.GetLatestRelease() != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("updates ticker never populated GetLatestRelease after Start")
}
