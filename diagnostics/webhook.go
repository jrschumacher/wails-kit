package diagnostics

import (
	"context"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jrschumacher/wails-kit/errors"
)

const (
	defaultWebhookTimeout  = 30 * time.Second
	defaultMaxRetries      = 3
	defaultInitialBackoff  = 1 * time.Second
)

// SubmitBundle sends a diagnostics bundle zip to the given webhook URL via
// multipart POST. The request includes X-App-Name and X-App-Version headers.
// If a bearer token is configured via WithWebhookToken, it is sent as an
// Authorization header. Retries with exponential backoff on transient failures
// (5xx status codes and network errors).
func (s *Service) SubmitBundle(ctx context.Context, bundlePath, webhookURL string) error {
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

		// Non-retryable client error (4xx)
		if statusCode >= 400 && statusCode < 500 {
			return errors.Newf(ErrBundleSubmit, "webhook returned status %d", statusCode)
		}

		// Retryable server error (5xx)
		lastErr = fmt.Errorf("webhook returned status %d", statusCode)
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

	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain body to allow connection reuse
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}
