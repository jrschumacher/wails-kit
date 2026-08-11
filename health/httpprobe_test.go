package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPProbeSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := HTTPProbe(srv.URL).Probe(context.Background()); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestHTTPProbeUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := HTTPProbe(srv.URL).Probe(context.Background()); err == nil {
		t.Fatal("expected a 500 response to fail the probe")
	}
}

func TestHTTPProbeCustomExpectStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	// Default (< 400) would accept 204 too, so assert the option is
	// actually wired by requiring something 204 does NOT satisfy.
	err := HTTPProbe(srv.URL, WithExpectStatus(func(code int) bool { return code == 200 })).Probe(context.Background())
	if err == nil {
		t.Fatal("expected WithExpectStatus override to reject a 204 when only 200 is accepted")
	}
}

func TestHTTPProbeUnreachable(t *testing.T) {
	srv := httptest.NewServer(nil)
	url := srv.URL
	srv.Close() // closed immediately: connection refused, no real network touched

	if err := HTTPProbe(url, WithTimeout(2*time.Second)).Probe(context.Background()); err == nil {
		t.Fatal("expected an error probing a closed listener")
	}
}

func TestHTTPProbeRespectsRequestTimeout(t *testing.T) {
	block := make(chan struct{})
	defer close(block)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	start := time.Now()
	err := HTTPProbe(srv.URL, WithTimeout(50*time.Millisecond)).Probe(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected the probe to time out against a handler that never responds")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected WithTimeout(50ms) to bound the request; took %s", elapsed)
	}
}

func TestHTTPProbeRespectsContextCancellation(t *testing.T) {
	block := make(chan struct{})
	defer close(block)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- HTTPProbe(srv.URL, WithTimeout(time.Hour)).Probe(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected cancellation to fail the probe")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("probe did not return promptly after context cancellation")
	}
}

func TestHTTPProbeCustomMethod(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := HTTPProbe(srv.URL, WithMethod(http.MethodHead)).Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodHead {
		t.Fatalf("expected HEAD, got %s", gotMethod)
	}
}
