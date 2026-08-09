package health

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// countingProbe counts how many times it was probed and always succeeds
// (or always fails, if fail is true). It never touches the network.
type countingProbe struct {
	n    *int32
	fail bool
}

func (p countingProbe) Probe(context.Context) error {
	atomic.AddInt32(p.n, 1)
	if p.fail {
		return context.DeadlineExceeded
	}
	return nil
}

// blockingProbe blocks until ctx is Done, then returns ctx.Err(). It
// exists to prove Start's shutdown doesn't wait for a fixed poll interval —
// it waits for exactly this in-flight probe to observe cancellation and
// return, which must happen promptly.
type blockingProbe struct{ started chan struct{} }

func (p blockingProbe) Probe(ctx context.Context) error {
	if p.started != nil {
		close(p.started)
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestStartProbesImmediatelyThenOnInterval(t *testing.T) {
	var n int32
	r := New(WithoutDefaultConnectivityCheck(), WithDefaultInterval(15*time.Millisecond))
	if _, err := r.Register(Check{Name: "api", Probe: countingProbe{n: &n}}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after its context was done")
	}

	if got := atomic.LoadInt32(&n); got < 2 {
		t.Fatalf("expected at least 2 probes (immediate + at least one tick) in 80ms at a 15ms interval, got %d", got)
	}
}

func TestStartCleanShutdownUnderCancellation(t *testing.T) {
	started := make(chan struct{})
	r := New(WithoutDefaultConnectivityCheck(), WithDefaultInterval(time.Hour))
	if _, err := r.Register(Check{Name: "slow", Probe: blockingProbe{started: started}}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("probe never started")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not shut down promptly after cancellation — the in-flight probe leaked past ctx cancellation")
	}
}

func TestStartIsNoopWhenAlreadyRunning(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck(), WithDefaultInterval(time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done1 := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done1)
	}()

	// Give the first Start a moment to mark itself running, then call it
	// again — it must return immediately rather than blocking or double
	// -scheduling checks.
	time.Sleep(20 * time.Millisecond)

	done2 := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done2)
	}()

	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("second concurrent Start call should have returned immediately as a no-op")
	}

	cancel()
	select {
	case <-done1:
	case <-time.After(2 * time.Second):
		t.Fatal("first Start did not shut down after cancellation")
	}
}

func TestRegisterWhileRunningJoinsTheLoop(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck(), WithDefaultInterval(15*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	var n int32
	if _, err := r.Register(Check{Name: "late", Probe: countingProbe{n: &n}}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&n) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if atomic.LoadInt32(&n) == 0 {
		t.Fatal("expected a check registered while Start is running to be probed")
	}
}

func TestUnregisterWhileRunningStopsProbing(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck(), WithDefaultInterval(10*time.Millisecond))

	var n int32
	if _, err := r.Register(Check{Name: "api", Probe: countingProbe{n: &n}}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()

	// Let it probe at least once, then remove it.
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&n) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	r.Unregister("api")
	countAtRemoval := atomic.LoadInt32(&n)

	time.Sleep(50 * time.Millisecond) // several more intervals' worth
	if got := atomic.LoadInt32(&n); got > countAtRemoval+1 {
		t.Fatalf("expected probing to stop after Unregister, count grew from %d to %d", countAtRemoval, got)
	}

	cancel()
	<-done
}
