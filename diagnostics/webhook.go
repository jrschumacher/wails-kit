package diagnostics

import (
	"context"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jrschumacher/wails-kit/v2/errors"
)

const (
	defaultWebhookTimeout = 30 * time.Second
	defaultMaxRetries     = 3
	defaultInitialBackoff = 1 * time.Second
	// maxWebhookBackoff caps exponential backoff so a large WithWebhookMaxRetries
	// value can't turn a failing endpoint into a multi-hour retry loop.
	maxWebhookBackoff = 30 * time.Second
	// maxDrainBytes bounds how much of a response body we'll read (and
	// discard) per attempt. Bundle uploads only care about the status code;
	// this stops a slow-drip or oversized response from a hostile or
	// misconfigured endpoint from holding the connection (and retry budget)
	// open for the full request timeout on every attempt.
	maxDrainBytes = 64 * 1024
)

// Submit is the consent-gated entry point for uploading a diagnostics
// bundle. It checks SubmissionConsent() and, only if the user has opted in
// (see SettingsGroup), uploads the bundle via the same mechanics as the
// former SubmitBundle. If consent has not been explicitly granted, Submit
// returns an error with code ErrConsentRequired instead of silently
// skipping the upload — a silent no-op here would be indistinguishable
// from a broken uploader, which is how crash reporting quietly never
// reports.
//
// Submit is the only exported way to upload a bundle; there is
// deliberately no consent-bypassing variant, so a caller can't wire a
// "send bundle" button straight to the network without going through the
// consent gate.
func (s *Service) Submit(ctx context.Context, bundlePath, webhookURL string) error {
	if !s.SubmissionConsent() {
		return errors.New(ErrConsentRequired, "diagnostics submission consent not granted", nil)
	}
	return s.submitBundle(ctx, bundlePath, webhookURL)
}

// submitBundle sends a diagnostics bundle zip to the given webhook URL via
// multipart POST. The request includes X-App-Name and X-App-Version headers.
// If a bearer token is configured via WithWebhookToken, it is sent as an
// Authorization header. Retries with exponential backoff on transient failures
// (5xx status codes and network errors).
//
// webhookURL must be https://, or plain http:// to a localhost destination
// (127.0.0.1/::1/"localhost") — e.g. for local development against a test
// receiver. Any other plaintext http:// destination is refused before any
// network call is made, since the upload carries bundle contents (which,
// depending on the consuming app, can include prompt/log text) and an
// optional bearer token in the clear.
func (s *Service) submitBundle(ctx context.Context, bundlePath, webhookURL string) error {
	if err := validateWebhookURL(webhookURL); err != nil {
		return err
	}

	token := s.webhookToken
	timeout := s.webhookTimeout
	if timeout == 0 {
		timeout = defaultWebhookTimeout
	}
	maxRetries := s.webhookMaxRetries
	if maxRetries == 0 {
		maxRetries = defaultMaxRetries
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(float64(defaultInitialBackoff) * math.Pow(2, float64(attempt-1)))
			if backoff > maxWebhookBackoff {
				backoff = maxWebhookBackoff
			}
			select {
			case <-ctx.Done():
				return errors.Wrap(ErrBundleSubmit, "context cancelled during retry", ctx.Err())
			case <-time.After(backoff):
			}
		}

		statusCode, err := s.doSubmit(ctx, bundlePath, webhookURL, token, timeout)
		if err == nil && statusCode >= 200 && statusCode < 300 {
			s.emit(EventBundleSubmitted, BundleSubmittedPayload{
				Path:       bundlePath,
				StatusCode: statusCode,
			})
			return nil
		}

		if err != nil {
			// Non-retryable errors (e.g., file not found, bad URL)
			if errors.IsCode(err, ErrBundleSubmit) {
				return err
			}
			// Network errors are retryable
			lastErr = err
			continue
		}

		// Server errors (5xx) are transient; retry.
		if statusCode >= 500 {
			lastErr = fmt.Errorf("webhook returned status %d", statusCode)
			continue
		}

		// Anything else non-2xx (4xx, or a 3xx redirect we deliberately did
		// not follow — see webhookClient) is treated as a hard, non-retryable
		// failure rather than something worth exhausting retries over.
		return errors.Newf(ErrBundleSubmit, "webhook returned status %d", statusCode)
	}

	return errors.Wrap(ErrBundleSubmit, "all retries exhausted", lastErr)
}

func (s *Service) doSubmit(ctx context.Context, bundlePath, webhookURL, token string, timeout time.Duration) (int, error) {
	f, err := os.Open(bundlePath)
	if err != nil {
		return 0, errors.Wrap(ErrBundleSubmit, "open bundle file", err)
	}
	defer func() { _ = f.Close() }()

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	// Write the multipart body in a goroutine to stream without buffering.
	go func() {
		part, err := mw.CreateFormFile("bundle", filepath.Base(bundlePath))
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_ = mw.Close()
		_ = pw.Close()
	}()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, pr)
	if err != nil {
		return 0, errors.Wrap(ErrBundleSubmit, "create request", err)
	}

	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-App-Name", s.appName)
	if s.appVersion != "" {
		req.Header.Set("X-App-Version", s.appVersion)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := s.webhookClient()

	resp, err := client.Do(req)
	if err != nil {
		// Not wrapped with ErrBundleSubmit: this is a transport-level error
		// (dial/timeout/TLS handshake, etc.) and must stay retryable. It may
		// contain the destination host, never bundle content or the response
		// body — nothing here reads response bytes into an error string.
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain (but cap) the body so the connection can be reused; the status
	// code is all that matters here and response bodies are never logged.
	_, _ = io.CopyN(io.Discard, resp.Body, maxDrainBytes)

	return resp.StatusCode, nil
}

// webhookClient returns the HTTP client used for bundle uploads, always
// with redirect-following disabled.
//
// Rationale: a compromised or merely misconfigured endpoint could otherwise
// respond with a redirect to an arbitrary destination — including a
// plaintext http:// one — and net/http would follow it by default, silently
// bypassing validateWebhookURL and re-sending the bearer token (and, for
// 307/308, the bundle body) wherever the redirect points. Refusing to
// follow means a redirect response is surfaced as-is (see the 3xx handling
// in submitBundle) instead of chased.
func (s *Service) webhookClient() *http.Client {
	base := s.httpClient
	if base == nil {
		base = http.DefaultClient
	}
	return &http.Client{
		Transport: base.Transport,
		Jar:       base.Jar,
		Timeout:   base.Timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// validateWebhookURL enforces TLS on webhook destinations: https:// is
// always allowed; plaintext http:// is allowed only to localhost
// (127.0.0.1, ::1, or the "localhost" hostname), so local development
// against a test receiver still works. Every other scheme, and every other
// http:// destination, is refused before any network call is made.
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.Wrap(ErrBundleSubmit, "parse webhook URL", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return errors.Newf(ErrBundleSubmit,
			"refusing plaintext http webhook destination %q: only https:// or localhost http:// destinations are allowed", u.Hostname())
	default:
		return errors.Newf(ErrBundleSubmit, "unsupported webhook URL scheme %q: must be https", u.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
