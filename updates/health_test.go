package updates

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/health"
)

func TestWithHealthRegistersCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := health.New(health.WithoutDefaultConnectivityCheck())

	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithSkipVerification(),
		WithGitHubAPIURL(srv.URL),
		WithHealth(r),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = svc

	snap := r.Snapshot()
	if len(snap.Checks) != 1 || snap.Checks[0].Name != HealthCheckName {
		t.Fatalf("expected %q registered, got %+v", HealthCheckName, snap.Checks)
	}
	if snap.Checks[0].Class != health.ClassProvider {
		t.Fatalf("expected ClassProvider, got %s", snap.Checks[0].Class)
	}

	r.Trigger(HealthCheckName)

	if got := r.Snapshot().Checks[0].State; got != health.StateHealthy {
		t.Fatalf("expected healthy against a live httptest server, got %s", got)
	}
}

func TestWithHealthOptionOrderDoesNotMatter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := health.New(health.WithoutDefaultConnectivityCheck())

	// WithHealth passed BEFORE WithGitHubAPIURL — the probe must still pick
	// up the API URL because it reads s.github at probe time, not option
	// -application time.
	_, err := NewService(
		WithHealth(r),
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithSkipVerification(),
		WithGitHubAPIURL(srv.URL),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r.Trigger(HealthCheckName)

	if got := r.Snapshot().Checks[0].State; got != health.StateHealthy {
		t.Fatalf("expected healthy regardless of option order, got %s", got)
	}
}

func TestWithHealthNilRegistryIsNoop(t *testing.T) {
	_, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithSkipVerification(),
		WithHealth(nil),
	)
	if err != nil {
		t.Fatalf("expected WithHealth(nil) to be a harmless no-op, got %v", err)
	}
}

func TestWithHealthDuplicateDoesNotFailConstruction(t *testing.T) {
	r := health.New(health.WithoutDefaultConnectivityCheck())
	if _, err := r.Register(health.Check{Name: HealthCheckName, Probe: health.HTTPProbe("http://example.invalid")}); err != nil {
		t.Fatal(err)
	}

	_, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithSkipVerification(),
		WithHealth(r),
	)
	if err != nil {
		t.Fatalf("expected a health-registry name conflict to be logged, not fatal to NewService: %v", err)
	}
}

func TestGithubReleasesProbeDownOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := health.New(health.WithoutDefaultConnectivityCheck())
	if _, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithSkipVerification(),
		WithGitHubAPIURL(srv.URL),
		WithHealth(r),
	); err != nil {
		t.Fatal(err)
	}

	r.Trigger(HealthCheckName)

	if got := r.Snapshot().Checks[0].State; got != health.StateDown {
		t.Fatalf("expected down on a 500 response, got %s", got)
	}
}

func TestGithubReleasesProbeHealthyOnRateLimitResponse(t *testing.T) {
	// A 403 (typical unauthenticated rate-limit response) still means
	// GitHub itself is reachable — only a 5xx or transport failure should
	// count as down.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	r := health.New(health.WithoutDefaultConnectivityCheck())
	if _, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithSkipVerification(),
		WithGitHubAPIURL(srv.URL),
		WithHealth(r),
	); err != nil {
		t.Fatal(err)
	}

	r.Trigger(HealthCheckName)

	if got := r.Snapshot().Checks[0].State; got != health.StateHealthy {
		t.Fatalf("expected a 403 (rate-limited, but reachable) to still count as healthy, got %s", got)
	}
}

func TestWithHealthCriticalAndInterval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := health.New(health.WithoutDefaultConnectivityCheck())
	if _, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithSkipVerification(),
		WithGitHubAPIURL(srv.URL),
		WithHealth(r, WithHealthCritical(true)),
	); err != nil {
		t.Fatal(err)
	}

	if !r.Snapshot().Checks[0].Critical {
		t.Fatal("expected WithHealthCritical(true) to mark the check critical")
	}
}
