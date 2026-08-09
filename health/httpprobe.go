package health

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// httpProbeConfig is built up by HTTPOption functions passed to HTTPProbe.
type httpProbeConfig struct {
	method       string
	timeout      time.Duration
	expectStatus func(code int) bool
	client       *http.Client
}

// HTTPOption configures an HTTPProbe.
type HTTPOption func(*httpProbeConfig)

// WithMethod overrides the HTTP method used by the probe. Default GET.
func WithMethod(method string) HTTPOption {
	return func(c *httpProbeConfig) { c.method = method }
}

// WithTimeout sets the probe's per-request timeout — the request is
// cancelled if it takes longer than d, regardless of the context passed to
// Probe. Default 10s. This is what keeps one hung endpoint from stalling a
// Registry indefinitely (each check still runs its own goroutine, but an
// unbounded probe would still leak that goroutine forever).
func WithTimeout(d time.Duration) HTTPOption {
	return func(c *httpProbeConfig) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// WithExpectStatus overrides the response-status predicate. Default: status
// < 400.
func WithExpectStatus(fn func(code int) bool) HTTPOption {
	return func(c *httpProbeConfig) { c.expectStatus = fn }
}

// WithHTTPClient overrides the *http.Client used by the probe. Default
// http.DefaultClient.
func WithHTTPClient(client *http.Client) HTTPOption {
	return func(c *httpProbeConfig) {
		if client != nil {
			c.client = client
		}
	}
}

// httpProbe is the Probe implementation returned by HTTPProbe.
type httpProbe struct {
	url string
	cfg httpProbeConfig
}

// HTTPProbe returns a Probe that issues an HTTP request to url and
// considers the endpoint reachable when the response status satisfies
// WithExpectStatus (default: status < 400). The request always carries a
// timeout (WithTimeout, default 10s) layered on top of whatever context
// Probe is called with, so a single request can never hang the calling
// goroutine indefinitely.
func HTTPProbe(url string, opts ...HTTPOption) Probe {
	cfg := httpProbeConfig{
		method:       http.MethodGet,
		timeout:      10 * time.Second,
		expectStatus: func(code int) bool { return code < 400 },
		client:       http.DefaultClient,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &httpProbe{url: url, cfg: cfg}
}

func (p *httpProbe) Probe(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, p.cfg.method, p.url, nil)
	if err != nil {
		return fmt.Errorf("health: build request: %w", err)
	}

	resp, err := p.cfg.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if !p.cfg.expectStatus(resp.StatusCode) {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// defaultConnectivityURL is probed by the registry's built-in connectivity
// check when no WithConnectivityProbe override is given. It is Google's
// well-known "captive portal" check endpoint: a bare GET that returns an
// empty 204 when there is a working path to the internet, and something
// else (a captive-portal login page, a DNS-hijack redirect, or a network
// error) otherwise. Override via WithConnectivityProbe for apps that don't
// want to depend on Google being reachable as their definition of "online".
const defaultConnectivityURL = "https://connectivitycheck.gstatic.com/generate_204"

// defaultConnectivityProbe builds the registry's out-of-the-box
// connectivity check: GET defaultConnectivityURL, expect exactly 204.
func defaultConnectivityProbe() Probe {
	return HTTPProbe(defaultConnectivityURL, WithExpectStatus(func(code int) bool { return code == 204 }))
}
