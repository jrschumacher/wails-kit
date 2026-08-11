package permissions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/v2/events"
)

func TestCheckNotDetermined(t *testing.T) {
	fp := newFakePlatform()
	fp.setSupported(Notifications, false) // not granted, never seen
	s := newService(fp)

	if got := s.Check(Notifications); got != StatusNotDetermined {
		t.Fatalf("Check() = %s, want %s", got, StatusNotDetermined)
	}
}

func TestCheckGranted(t *testing.T) {
	fp := newFakePlatform()
	fp.setSupported(Notifications, true)
	s := newService(fp)

	if got := s.Check(Notifications); got != StatusGranted {
		t.Fatalf("Check() = %s, want %s", got, StatusGranted)
	}
}

func TestCheckDeniedAfterSeen(t *testing.T) {
	fp := newFakePlatform()
	fp.setSupported(Notifications, false)
	s := newService(fp)

	// Not yet asked: not_determined.
	if got := s.Check(Notifications); got != StatusNotDetermined {
		t.Fatalf("before Request: Check() = %s, want %s", got, StatusNotDetermined)
	}

	// Ask (still not granted per the fake): now denied, not
	// not_determined, because the app must not re-prompt after asking.
	if _, err := s.Request(context.Background(), Notifications); err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if got := s.Check(Notifications); got != StatusDenied {
		t.Fatalf("after Request: Check() = %s, want %s", got, StatusDenied)
	}
}

func TestCheckUnsupportedKind(t *testing.T) {
	fp := newFakePlatform() // nothing marked supported
	s := newService(fp)

	for _, k := range []Kind{Notifications, Accessibility, FullDiskAccess} {
		if got := s.Check(k); got != StatusUnsupported {
			t.Fatalf("Check(%s) = %s, want %s", k, got, StatusUnsupported)
		}
	}
}

func TestCheckUnsupportedNeverConflatedWithDenied(t *testing.T) {
	fp := newFakePlatform() // Notifications never marked supported
	s := newService(fp)

	// Calling Request first (marks "seen") must not turn a genuinely
	// unsupported Kind into Denied — unsupported always wins.
	_, _ = s.Request(context.Background(), Notifications)
	if got := s.Check(Notifications); got != StatusUnsupported {
		t.Fatalf("Check() = %s, want %s (unsupported must never become denied)", got, StatusUnsupported)
	}
}

func TestRequestMarksSeenEvenOnError(t *testing.T) {
	fp := newFakePlatform()
	fp.setSupported(Notifications, false)
	fp.requestErr[Notifications] = errors.New("boom")
	s := newService(fp)

	if _, err := s.Request(context.Background(), Notifications); err == nil {
		t.Fatal("expected Request() to surface the platform error")
	}
	if got := s.Check(Notifications); got != StatusDenied {
		t.Fatalf("Check() after failed Request = %s, want %s", got, StatusDenied)
	}
}

func TestRequestAlreadyCancelledContext(t *testing.T) {
	fp := newFakePlatform()
	fp.setSupported(Notifications, false)
	s := newService(fp)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status, err := s.Request(ctx, Notifications)
	if err == nil {
		t.Fatal("expected context.Canceled error")
	}
	if status != StatusNotDetermined {
		t.Fatalf("status = %s, want %s (Request never ran, so nothing was asked)", status, StatusNotDetermined)
	}
	fp.mu.Lock()
	calls := len(fp.requestCalls)
	fp.mu.Unlock()
	if calls != 0 {
		t.Fatalf("expected the platform request to never be called for an already-cancelled context, got %d calls", calls)
	}
}

// TestRequestContextCancelledMidFlight is the "requesting is asynchronous
// and may never return" case: the platform call blocks (simulating a user
// ignoring the OS dialog) and Request must return promptly once ctx is
// cancelled, rather than blocking until the platform call itself resolves.
func TestRequestContextCancelledMidFlight(t *testing.T) {
	fp := newFakePlatform()
	fp.setSupported(Notifications, false)
	fp.requestBlock = make(chan struct{}) // never closed during this test

	s := newService(fp)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	status, err := s.Request(ctx, Notifications)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected ctx deadline error")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Request blocked for %s past context cancellation — it must return promptly", elapsed)
	}
	// The platform call was in fact started (and is still running,
	// leaked, in the background — documented in AGENTS.md); "seen" is
	// still recorded because the app did ask.
	if status != StatusDenied {
		t.Fatalf("status = %s, want %s (Request was called, even though it didn't resolve in time)", status, StatusDenied)
	}

	close(fp.requestBlock) // let the leaked goroutine finish so it doesn't outlive the test
}

func TestOpenSystemSettingsMarksSeen(t *testing.T) {
	fp := newFakePlatform()
	fp.setSupported(Accessibility, false)
	s := newService(fp)

	if got := s.Check(Accessibility); got != StatusNotDetermined {
		t.Fatalf("before OpenSystemSettings: Check() = %s, want %s", got, StatusNotDetermined)
	}
	if err := s.OpenSystemSettings(Accessibility); err != nil {
		t.Fatalf("OpenSystemSettings() error = %v", err)
	}
	if got := s.Check(Accessibility); got != StatusDenied {
		t.Fatalf("after OpenSystemSettings: Check() = %s, want %s", got, StatusDenied)
	}
}

func TestEventChangedOnlyOnTransition(t *testing.T) {
	fp := newFakePlatform()
	fp.setSupported(Notifications, false)
	mem := events.NewMemoryEmitter()
	s := newService(fp, WithEmitter(events.NewEmitter(mem)))

	s.Check(Notifications) // not_determined: first observation, should emit
	s.Check(Notifications) // still not_determined: no new emission
	s.Check(Notifications) // still not_determined: no new emission

	if got := mem.Count(); got != 1 {
		t.Fatalf("expected exactly one emission for the first observation, got %d", got)
	}

	fp.setSupported(Notifications, true)
	s.Check(Notifications) // now granted: transition, should emit again

	if got := mem.Count(); got != 2 {
		t.Fatalf("expected a second emission on transition to granted, got %d", got)
	}

	last := mem.Last()
	if last == nil || last.Name != EventChanged {
		t.Fatalf("expected last event %q, got %+v", EventChanged, last)
	}
	payload, ok := last.Data.(ChangedPayload)
	if !ok {
		t.Fatalf("expected ChangedPayload, got %T", last.Data)
	}
	if payload.Kind != Notifications || payload.Status != StatusGranted {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestTextFallsBackWithoutLocalizer(t *testing.T) {
	s := newService(newFakePlatform())
	if got := s.Text(RationaleNotifications); got != RationaleNotifications.Other {
		t.Fatalf("Text() = %q, want the literal Other fallback %q", got, RationaleNotifications.Other)
	}
}

func TestBindingDelegatesToService(t *testing.T) {
	fp := newFakePlatform()
	fp.setSupported(Notifications, true)
	fp.setSupported(FullDiskAccess, false)
	s := newService(fp)
	b := s.Binding()

	if got := b.Check(Notifications); got != StatusGranted {
		t.Fatalf("Binding.Check() = %s, want %s", got, StatusGranted)
	}
	if err := b.OpenSystemSettings(FullDiskAccess); err != nil {
		t.Fatalf("Binding.OpenSystemSettings() error = %v", err)
	}
	if got, err := b.Request(FullDiskAccess); err != nil || got != StatusDenied {
		t.Fatalf("Binding.Request() = (%s, %v), want (%s, nil)", got, err, StatusDenied)
	}
}
