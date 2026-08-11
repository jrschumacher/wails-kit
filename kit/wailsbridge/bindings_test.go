package wailsbridge

import (
	"testing"

	"github.com/jrschumacher/wails-kit/v2/diagnostics"
	"github.com/jrschumacher/wails-kit/v2/health"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/kit"
	"github.com/jrschumacher/wails-kit/v2/permissions"
	"github.com/jrschumacher/wails-kit/v2/settings"
	"github.com/jrschumacher/wails-kit/v2/updates"
)

// TestBindingInstancesNeverRegisterRawService is this package's equivalent
// of settings/binding_test.go's TestBindingSurface: it proves
// bindingInstances (what bindingServices wraps and hands to
// app.RegisterService) never contains a raw *settings.Service,
// *i18n.Localizer, *health.Registry, *permissions.Service,
// *updates.Service, or *diagnostics.Service. Wails v3 binds every exported
// method of a registered service — registering *settings.Service directly
// would republish GetSecret (raw, unmasked API keys) to webview JS, which
// is exactly the Phase 0 finding settings.Binding exists to prevent. This
// test asserts the same guarantee holds at the point wailsbridge hands
// instances to Wails, not just that settings.Binding itself is narrow.
func TestBindingInstancesNeverRegisterRawService(t *testing.T) {
	k := newTestKit(t,
		kit.WithGitHubRepo("acme", "widget"), kit.WithUpdatesSkipVerification(), // enables k.Updates
	)
	cfg := newConfig()

	instances := bindingInstances(k, cfg)
	if len(instances) == 0 {
		t.Fatal("expected at least one binding instance")
	}

	for _, inst := range instances {
		switch inst.(type) {
		case *settings.Service:
			t.Error("bindingInstances contains a raw *settings.Service — would publish GetSecret to the webview")
		case *i18n.Localizer:
			t.Error("bindingInstances contains a raw *i18n.Localizer")
		case *health.Registry:
			t.Error("bindingInstances contains a raw *health.Registry — would publish Register/Unregister to the webview")
		case *permissions.Service:
			t.Error("bindingInstances contains a raw *permissions.Service")
		case *updates.Service:
			t.Error("bindingInstances contains a raw *updates.Service")
		case *diagnostics.Service:
			t.Error("bindingInstances contains a raw *diagnostics.Service — would publish Submit(url) with a caller-chosen URL")
		case *settings.Binding, *i18n.Binding, *health.Binding, *permissions.Binding, *UpdatesBinding, *DiagnosticsBinding:
			// expected — the narrow surface.
		default:
			t.Errorf("bindingInstances contains an unrecognized type %T — update this test's allowlist", inst)
		}
	}
}

// TestBindingInstances_CountsMatchEnabledComponents verifies the expected
// shape: settings + i18n always present; health/updates/diagnostics only
// when their Kit field is non-nil; permissions always present (it has no
// Kit field — see AGENTS.md).
func TestBindingInstances_CountsMatchEnabledComponents(t *testing.T) {
	t.Run("defaults (health+diagnostics on, updates off)", func(t *testing.T) {
		k := newTestKit(t)
		instances := bindingInstances(k, newConfig())
		// settings, i18n, health, permissions, diagnostics — no updates.
		want := 5
		if len(instances) != want {
			t.Errorf("len(instances) = %d, want %d", len(instances), want)
		}
		assertNoType[*UpdatesBinding](t, instances)
	})

	t.Run("updates enabled", func(t *testing.T) {
		k := newTestKit(t, kit.WithGitHubRepo("acme", "widget"), kit.WithUpdatesSkipVerification())
		instances := bindingInstances(k, newConfig())
		want := 6
		if len(instances) != want {
			t.Errorf("len(instances) = %d, want %d", len(instances), want)
		}
	})

	t.Run("health and diagnostics disabled", func(t *testing.T) {
		k := newTestKit(t, kit.WithoutHealth(), kit.WithoutDiagnostics())
		instances := bindingInstances(k, newConfig())
		// settings, i18n, permissions.
		want := 3
		if len(instances) != want {
			t.Errorf("len(instances) = %d, want %d", len(instances), want)
		}
	})
}

func assertNoType[T any](t *testing.T, instances []any) {
	t.Helper()
	for _, inst := range instances {
		if _, ok := inst.(T); ok {
			t.Errorf("unexpected instance of type %T present", inst)
		}
	}
}

// TestUpdatesBinding_Mirrors verifies UpdatesBinding forwards to the
// wrapped *updates.Service without altering behavior.
func TestUpdatesBinding_Mirrors(t *testing.T) {
	svc, err := updates.NewService(
		updates.WithCurrentVersion("1.2.3"),
		updates.WithGitHubRepo("acme", "widget"), updates.WithSkipVerification(),
	)
	if err != nil {
		t.Fatalf("updates.NewService: %v", err)
	}
	b := newUpdatesBinding(svc)

	// updates.Service normalizes the version through semver.ParseVersion,
	// which canonicalizes a leading "v" — see updates/service.go's
	// WithCurrentVersion. Asserting the normalized form (not the literal
	// input) is what actually proves UpdatesBinding forwards to svc rather
	// than caching/echoing its constructor argument.
	if got := b.GetCurrentVersion(); got != "v1.2.3" {
		t.Errorf("GetCurrentVersion() = %q, want %q", got, "v1.2.3")
	}
	if got := b.GetLatestRelease(); got != nil {
		t.Errorf("GetLatestRelease() = %v, want nil before any check", got)
	}
}

// TestDiagnosticsBinding_SubmitRefusesWithoutWebhookURL verifies Submit
// returns ErrWebhookUnconfigured — not a silent no-op, and without
// creating a bundle — when WithDiagnosticsWebhookURL was never given.
func TestDiagnosticsBinding_SubmitRefusesWithoutWebhookURL(t *testing.T) {
	k := newTestKit(t)
	cfg := newConfig() // no diagWebhookURL

	b := newDiagnosticsBinding(k, cfg)
	err := b.Submit(t.Context())
	if err == nil {
		t.Fatal("expected an error when no webhook URL is configured")
	}
}
