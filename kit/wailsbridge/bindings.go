package wailsbridge

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/jrschumacher/wails-kit/v2/appearance"
	"github.com/jrschumacher/wails-kit/v2/diagnostics"
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/health"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/kit"
	"github.com/jrschumacher/wails-kit/v2/permissions"
	"github.com/jrschumacher/wails-kit/v2/settings"
	"github.com/jrschumacher/wails-kit/v2/updates"
)

// bindingServices builds the frontend-safe binding surface for k: exactly
// one application.Service per enabled component, wrapping the narrowest
// type available — never a raw *settings.Service, *i18n.Localizer,
// *health.Registry, *permissions.Service, *updates.Service, or
// *diagnostics.Service. Wails v3 binds every exported method of a
// registered service, so registering one of those directly would publish
// whatever backend-only methods it has (settings.Service.GetSecret is the
// canonical example — a real Phase 0 finding that settings/binding_test.go's
// TestBindingSurface exists to prevent). This function is this package's
// equivalent guarantee for the components it wires in.
//
// settings, i18n, health, and permissions already ship a narrow *Binding
// type (settings.Binding, i18n.Binding, health.Binding,
// permissions.Binding) — this just calls .Binding() on each. updates and
// diagnostics do not (see AGENTS.md, "updates and diagnostics have no
// Binding type"), so UpdatesBinding and DiagnosticsBinding below supply
// one, deliberately narrower than a straight method-for-method mirror
// where the underlying Service method takes a caller-chosen path or URL —
// see DiagnosticsBinding's doc comment.
//
// The type switch below (rather than a generic loop over bindingInstances'
// []any) is what keeps application.NewService's generic instantiation
// concrete per type — Go cannot infer NewService[T]'s T from a value
// statically typed any, only from each case's typed variable.
func bindingServices(k *kit.Kit, cfg *config) []application.Service {
	instances := bindingInstances(k, cfg)
	out := make([]application.Service, 0, len(instances))
	for _, inst := range instances {
		switch v := inst.(type) {
		case *settings.Binding:
			out = append(out, application.NewService(v))
		case *i18n.Binding:
			out = append(out, application.NewService(v))
		case *health.Binding:
			out = append(out, application.NewService(v))
		case *permissions.Binding:
			out = append(out, application.NewService(v))
		case *appearance.Binding:
			out = append(out, application.NewService(v))
		case *UpdatesBinding:
			out = append(out, application.NewService(v))
		case *DiagnosticsBinding:
			out = append(out, application.NewService(v))
		}
	}
	return out
}

// bindingInstances builds the raw instances bindingServices wraps, kept
// separate so tests can assert on their concrete types directly —
// application.Service.instance is unexported, so once wrapped there is no
// way to inspect what got registered. See bindings_test.go,
// TestBindingInstancesNeverRegisterRawService.
func bindingInstances(k *kit.Kit, cfg *config) []any {
	out := []any{
		k.Settings.Binding(),
		k.I18n.Binding(),
		// appearance was missing here until a consumer generated bindings
		// and found the import unresolvable. A theme toggle in the UI is
		// the primary reason appearance has a Binding at all, so leaving it
		// unregistered made the package unreachable from the frontend
		// through the standard wiring.
		k.Appearance.Binding(),
	}

	if k.Health != nil {
		out = append(out, k.Health.Binding())
	}

	permOpts := append([]permissions.Option{
		permissions.WithEmitter(k.Events),
		permissions.WithLocalizer(k.I18n),
	}, cfg.permissionsOpts...)
	permSvc := permissions.NewService(permOpts...)
	out = append(out, permSvc.Binding())

	if k.Updates != nil {
		out = append(out, newUpdatesBinding(k.Updates))
	}

	if k.Diag != nil {
		out = append(out, newDiagnosticsBinding(k, cfg))
	}

	return out
}

// UpdatesBinding is the frontend-safe surface of *updates.Service. As of
// this WP, updates.Service has no caller-supplied path/URL parameters on
// any exported method — every one is either niladic or takes only a
// context.Context — so this is a 1:1 mirror rather than a narrowed subset.
// It still exists (rather than registering *updates.Service directly)
// because a mirror decouples the webview-exposed surface from whatever
// updates.Service grows next: a future method with a wider blast radius
// (e.g. one accepting a caller-chosen path) lands here only if someone
// deliberately adds it, not automatically via reflection over every
// exported method.
type UpdatesBinding struct {
	svc *updates.Service
}

func newUpdatesBinding(svc *updates.Service) *UpdatesBinding {
	return &UpdatesBinding{svc: svc}
}

// CheckForUpdate reports the latest release, if any. See
// updates.Service.CheckForUpdate.
func (b *UpdatesBinding) CheckForUpdate(ctx context.Context) (*updates.Release, error) {
	return b.svc.CheckForUpdate(ctx)
}

// DownloadUpdate downloads and verifies the latest checked release. See
// updates.Service.DownloadUpdate.
func (b *UpdatesBinding) DownloadUpdate(ctx context.Context) (string, error) {
	return b.svc.DownloadUpdate(ctx)
}

// ApplyUpdate re-verifies and applies the most recently downloaded update.
// See updates.Service.ApplyUpdate.
func (b *UpdatesBinding) ApplyUpdate(ctx context.Context) error {
	return b.svc.ApplyUpdate(ctx)
}

// GetCurrentVersion returns the running app's version. See
// updates.Service.GetCurrentVersion.
func (b *UpdatesBinding) GetCurrentVersion() string {
	return b.svc.GetCurrentVersion()
}

// GetLatestRelease returns the most recently checked release, if any. See
// updates.Service.GetLatestRelease.
func (b *UpdatesBinding) GetLatestRelease() *updates.Release {
	return b.svc.GetLatestRelease()
}

// DiagnosticsBinding is the frontend-safe surface of *diagnostics.Service.
// diagnostics.Service.CreateBundle(ctx, outputDir) and
// Submit(ctx, bundlePath, webhookURL) both take a caller-chosen filesystem
// path or URL — safe from Go code, where the app chooses those values, but
// not safe as a direct webview binding: an arbitrary outputDir/bundlePath
// is a path-traversal read/write primitive, and an arbitrary webhookURL
// turns the "send diagnostics" button into an exfiltration primitive to
// wherever the frontend (or anything that can drive it) points it. Neither
// parameter is meaningful to a frontend anyway — a user clicking "Create
// bundle" or "Submit" has no reason to choose a directory or a URL, since
// diagnostics bundles always go to one place per app.
//
// This binding fixes both: CreateBundle always writes into k.Dirs.Cache(),
// and Submit always uploads to the URL configured via
// WithDiagnosticsWebhookURL(Attach/Services options) — never a
// caller-supplied one. Submit creates a fresh bundle internally rather
// than accepting a bundlePath, so there is no path parameter at all on the
// webview-facing surface.
type DiagnosticsBinding struct {
	svc        *diagnostics.Service
	bundleDir  string
	webhookURL string
}

func newDiagnosticsBinding(k *kit.Kit, cfg *config) *DiagnosticsBinding {
	return &DiagnosticsBinding{
		svc:        k.Diag,
		bundleDir:  k.Dirs.Cache(),
		webhookURL: cfg.diagWebhookURL,
	}
}

// CreateBundle creates a diagnostics bundle in the fixed cache directory
// and returns its path (e.g. for a "reveal in Finder"-style affordance).
// See diagnostics.Service.CreateBundle.
func (b *DiagnosticsBinding) CreateBundle(ctx context.Context) (string, error) {
	return b.svc.CreateBundle(ctx, b.bundleDir)
}

// SubmissionConsent reports whether the user has opted into diagnostics
// submission. See diagnostics.Service.SubmissionConsent.
func (b *DiagnosticsBinding) SubmissionConsent() bool {
	return b.svc.SubmissionConsent()
}

// Submit creates a fresh bundle and uploads it to the configured webhook
// URL (see WithDiagnosticsWebhookURL), if the user has granted consent
// (see SubmissionConsent / diagnostics.Service.Submit). Returns
// ErrWebhookUnconfigured, without creating a bundle, if no URL was
// configured — a silent no-op here would be as misleading as the
// consent-absent no-op diagnostics.Service.Submit itself already refuses
// to be.
func (b *DiagnosticsBinding) Submit(ctx context.Context) error {
	if b.webhookURL == "" {
		return errors.New(ErrWebhookUnconfigured, "wailsbridge: diagnostics webhook URL not configured", nil)
	}
	bundlePath, err := b.svc.CreateBundle(ctx, b.bundleDir)
	if err != nil {
		return err
	}
	return b.svc.Submit(ctx, bundlePath, b.webhookURL)
}

// GetSystemInfo returns system and application metadata for display. See
// diagnostics.Service.GetSystemInfo.
func (b *DiagnosticsBinding) GetSystemInfo() diagnostics.SystemInfo {
	return b.svc.GetSystemInfo()
}
