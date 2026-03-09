package diagnostics

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/events"
)

func TestSubmitBundle(t *testing.T) {
	// Helper to create a dummy bundle zip file.
	createDummyBundle := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "test-bundle.zip")
		if err := os.WriteFile(path, []byte("fake zip content"), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("successful submission", func(t *testing.T) {
		bundlePath := createDummyBundle(t)

		var receivedAppName string
		var receivedContentType string
		var receivedBody []byte

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAppName = r.Header.Get("X-App-Name")
			receivedContentType = r.Header.Get("Content-Type")

			mediaType, params, err := mime.ParseMediaType(receivedContentType)
			if err != nil {
				t.Errorf("failed to parse content type: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if mediaType != "multipart/form-data" {
				t.Errorf("expected multipart/form-data, got %s", mediaType)
			}

			mr := multipart.NewReader(r.Body, params["boundary"])
			part, err := mr.NextPart()
			if err != nil {
				t.Errorf("failed to read part: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			receivedBody, _ = io.ReadAll(part)

			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		mem := events.NewMemoryEmitter()
		emitter := events.NewEmitter(mem)

		svc, err := NewService(
			WithAppName("test-app"),
			WithVersion("1.0.0"),
			WithEmitter(emitter),
		)
		if err != nil {
			t.Fatal(err)
		}

		err = svc.SubmitBundle(context.Background(), bundlePath, srv.URL)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if receivedAppName != "test-app" {
			t.Errorf("expected X-App-Name test-app, got %s", receivedAppName)
		}
		if string(receivedBody) != "fake zip content" {
			t.Errorf("unexpected body: %s", receivedBody)
		}

		// Check event
		if mem.Count() != 1 {
			t.Fatalf("expected 1 event, got %d", mem.Count())
		}
		last := mem.Last()
		if last.Name != EventBundleSubmitted {
			t.Errorf("expected event %s, got %s", EventBundleSubmitted, last.Name)
		}
		payload, ok := last.Data.(BundleSubmittedPayload)
		if !ok {
			t.Fatal("expected BundleSubmittedPayload")
		}
		if payload.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", payload.StatusCode)
		}
	})

	t.Run("sends bearer token", func(t *testing.T) {
		bundlePath := createDummyBundle(t)
		var receivedAuth string

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		svc, err := NewService(
			WithAppName("test-app"),
			WithWebhookToken("my-secret-token"),
		)
		if err != nil {
			t.Fatal(err)
		}

		err = svc.SubmitBundle(context.Background(), bundlePath, srv.URL)
		if err != nil {
			t.Fatal(err)
		}

		if receivedAuth != "Bearer my-secret-token" {
			t.Errorf("expected Bearer token, got %s", receivedAuth)
		}
	})

	t.Run("sends app version header", func(t *testing.T) {
		bundlePath := createDummyBundle(t)
		var receivedVersion string

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedVersion = r.Header.Get("X-App-Version")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		svc, err := NewService(
			WithAppName("test-app"),
			WithVersion("2.5.0"),
		)
		if err != nil {
			t.Fatal(err)
		}

		err = svc.SubmitBundle(context.Background(), bundlePath, srv.URL)
		if err != nil {
			t.Fatal(err)
		}

		if receivedVersion != "2.5.0" {
			t.Errorf("expected X-App-Version 2.5.0, got %s", receivedVersion)
		}
	})

	t.Run("retries on 5xx", func(t *testing.T) {
		bundlePath := createDummyBundle(t)
		var attempts atomic.Int32

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempts.Add(1)
			if n < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		svc, err := NewService(
			WithAppName("test-app"),
			WithWebhookMaxRetries(3),
		)
		if err != nil {
			t.Fatal(err)
		}

		err = svc.SubmitBundle(context.Background(), bundlePath, srv.URL)
		if err != nil {
			t.Fatalf("expected success after retries, got: %v", err)
		}

		if attempts.Load() != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts.Load())
		}
	})

	t.Run("fails on 4xx without retry", func(t *testing.T) {
		bundlePath := createDummyBundle(t)
		var attempts atomic.Int32

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			w.WriteHeader(http.StatusForbidden)
		}))
		defer srv.Close()

		svc, err := NewService(WithAppName("test-app"))
		if err != nil {
			t.Fatal(err)
		}

		err = svc.SubmitBundle(context.Background(), bundlePath, srv.URL)
		if err == nil {
			t.Fatal("expected error for 403")
		}

		if attempts.Load() != 1 {
			t.Errorf("expected 1 attempt (no retry on 4xx), got %d", attempts.Load())
		}
	})

	t.Run("exhausts retries on persistent 5xx", func(t *testing.T) {
		bundlePath := createDummyBundle(t)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()

		svc, err := NewService(
			WithAppName("test-app"),
			WithWebhookMaxRetries(2),
		)
		if err != nil {
			t.Fatal(err)
		}

		err = svc.SubmitBundle(context.Background(), bundlePath, srv.URL)
		if err == nil {
			t.Fatal("expected error after exhausting retries")
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		bundlePath := createDummyBundle(t)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		svc, err := NewService(
			WithAppName("test-app"),
			WithWebhookMaxRetries(5),
		)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err = svc.SubmitBundle(ctx, bundlePath, srv.URL)
		if err == nil {
			t.Fatal("expected error from context cancellation")
		}
	})

	t.Run("fails for missing bundle file", func(t *testing.T) {
		svc, err := NewService(WithAppName("test-app"))
		if err != nil {
			t.Fatal(err)
		}

		err = svc.SubmitBundle(context.Background(), "/nonexistent/bundle.zip", "http://localhost")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}
