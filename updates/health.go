package updates

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jrschumacher/wails-kit/v2/health"
)

// HealthCheckName is the name updates registers with a health.Registry via
// WithHealth.
const HealthCheckName = "github-releases"

// healthConfig is built up by HealthOption functions passed to WithHealth.
type healthConfig struct {
	critical bool
	interval time.Duration
}

// HealthOption configures the health.Check WithHealth registers.
type HealthOption func(*healthConfig)

// WithHealthCritical marks the registered check as participating in the
// registry's overall status (health.Check.Critical). Off by default: GitHub
// being unreachable is rarely something an app should degrade over, since
// checking for updates is a background convenience, not something the user
// is blocked on.
func WithHealthCritical(critical bool) HealthOption {
	return func(c *healthConfig) { c.critical = critical }
}

// WithHealthInterval overrides the registered check's probing interval.
// Zero (the default) defers to the registry's own default interval.
func WithHealthInterval(d time.Duration) HealthOption {
	return func(c *healthConfig) { c.interval = d }
}

// WithHealth registers a health.Check for the GitHub Releases API with r, so
// apps get "is GitHub reachable" for free instead of every app re-probing it
// themselves — the pattern docs/v2-roadmap.md WP-15 calls out explicitly:
// "the updater registering its own reachability check without the app
// having to know."
//
// Class is health.ClassProvider: GitHub being unreachable is a third-party
// problem ("try again later"), not "your app is broken" (health.ClassBackend)
// and not "you have no network at all" (health.ClassConnectivity — the
// registry's own built-in check covers that separately).
//
// The registered probe reads s's GitHub configuration (API URL, HTTP
// client) at *probe time*, not at option-application time, so WithHealth
// may be passed to NewService before or after WithGitHubRepo/
// WithGitHubAPIURL/WithHTTPClient in any order.
//
// r must not be nil (a nil r is a no-op, not a panic — makes it safe to
// pass a possibly-absent registry straight through from app wiring code).
// If registration fails (most likely: a check named "github-releases" is
// already registered), the failure is logged and NewService still
// succeeds — a health-registry conflict is not a reason to refuse to
// construct the updater.
func WithHealth(r *health.Registry, opts ...HealthOption) ServiceOption {
	return func(s *Service) {
		if r == nil {
			return
		}
		cfg := healthConfig{}
		for _, opt := range opts {
			opt(&cfg)
		}
		_, err := r.Register(health.Check{
			Name:     HealthCheckName,
			Class:    health.ClassProvider,
			Critical: cfg.critical,
			Interval: cfg.interval,
			Probe:    &githubReleasesProbe{svc: s},
		})
		if err != nil {
			slog.Warn("updates: failed to register health check", "error", err)
		}
	}
}

// githubReleasesProbe adapts Service's GitHub configuration to health.Probe.
// It reads svc.github fresh on every Probe call rather than capturing it
// once — see WithHealth's doc comment on why that matters for option
// ordering.
type githubReleasesProbe struct {
	svc *Service
}

// Probe issues a bounded GET against the configured GitHub API base URL
// (default https://api.github.com) and treats any response under 500 as
// "GitHub is reachable" — a 401/403/429 still means the network path and
// the service itself are up; only a server error or a transport failure
// (DNS, connection refused, timeout) counts as down. Delegates to
// health.HTTPProbe for the actual request so the timeout/cancellation
// behavior matches every other HTTP-based check in the kit.
func (p *githubReleasesProbe) Probe(ctx context.Context) error {
	base := "https://api.github.com"
	var client *http.Client
	if g := p.svc.github; g != nil {
		if g.apiURL != "" {
			base = g.apiURL
		}
		client = g.client
	}

	opts := []health.HTTPOption{
		health.WithTimeout(10 * time.Second),
		health.WithExpectStatus(func(code int) bool { return code < 500 }),
	}
	if client != nil {
		opts = append(opts, health.WithHTTPClient(client))
	}

	return health.HTTPProbe(base, opts...).Probe(ctx)
}
