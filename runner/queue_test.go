package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
)

// fakeClock is a mutable, injectable clock for tests — per
// docs/v2-roadmap.md WP-40: "test it by injecting a clock rather than
// sleeping in tests". Tests advance it explicitly to simulate the passage
// of time (including sleep/wake gaps) without ever calling time.Sleep.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{t: start} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTestQueue(t *testing.T, p Profile, clock *fakeClock, opts ...Option) *Queue {
	t.Helper()
	allOpts := append([]Option{WithClock(clock.Now)}, opts...)
	q, err := New(p, allOpts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return q
}

func TestNewRequiresStoreForPersistStore(t *testing.T) {
	p := Durable() // Persistence == PersistStore
	_, err := New(p)
	if err == nil {
		t.Fatal("New with PersistStore and no WithStore: want error, got nil")
	}
	if errors.GetCode(err) != ErrStoreRequired {
		t.Fatalf("error code = %v, want ErrStoreRequired", errors.GetCode(err))
	}
}

func TestNewDefaultsInMemoryStoreForPersistNone(t *testing.T) {
	q, err := New(Ephemeral())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if q.store == nil {
		t.Fatal("PersistNone queue has no store at all")
	}
}

func TestBasicJobRunsAndCompletes(t *testing.T) {
	clock := newFakeClock(time.Now())
	q := newTestQueue(t, Ephemeral(), clock)

	var gotName string
	q.Handle("greet", func(_ context.Context, job Job) error {
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return err
		}
		gotName = p.Name
		return nil
	})

	id, err := q.Enqueue(context.Background(), "greet", map[string]string{"name": "ada"})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if id == "" {
		t.Fatal("Enqueue returned an empty ID")
	}

	q.processTick()
	q.wg.Wait()

	if gotName != "ada" {
		t.Fatalf("handler saw payload name = %q, want ada", gotName)
	}
}

func TestEnqueueReturnsUniqueIDs(t *testing.T) {
	clock := newFakeClock(time.Now())
	q := newTestQueue(t, Ephemeral(), clock)
	q.Handle("t", func(context.Context, Job) error { return nil })

	id1, err := q.Enqueue(context.Background(), "t", nil)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	id2, err := q.Enqueue(context.Background(), "t", nil)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("two Enqueue calls without an idempotency key returned the same ID %q", id1)
	}
}

func TestIdempotencyKeyDedupWhilePending(t *testing.T) {
	clock := newFakeClock(time.Now())
	q := newTestQueue(t, Ephemeral(), clock)

	id1, err := q.Enqueue(context.Background(), "t", nil, WithIdempotencyKey("k1"))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	id2, err := q.Enqueue(context.Background(), "t", nil, WithIdempotencyKey("k1"))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("duplicate idempotency key returned a new ID: %q != %q", id1, id2)
	}
}

func TestIdempotencyKeyReleasedAfterCompletion(t *testing.T) {
	clock := newFakeClock(time.Now())
	q := newTestQueue(t, Ephemeral(), clock)
	q.Handle("t", func(context.Context, Job) error { return nil })

	id1, err := q.Enqueue(context.Background(), "t", nil, WithIdempotencyKey("k1"))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	q.processTick()
	q.wg.Wait()

	id2, err := q.Enqueue(context.Background(), "t", nil, WithIdempotencyKey("k1"))
	if err != nil {
		t.Fatalf("second Enqueue: %v", err)
	}
	if id1 == id2 {
		t.Fatal("idempotency key was not released after the first job completed")
	}
}

func TestBackpressureErrQueueFull(t *testing.T) {
	clock := newFakeClock(time.Now())
	p := Ephemeral()
	p.MaxQueueDepth = 2
	q := newTestQueue(t, p, clock)

	for i := 0; i < 2; i++ {
		if _, err := q.Enqueue(context.Background(), "t", nil); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	_, err := q.Enqueue(context.Background(), "t", nil)
	if err == nil {
		t.Fatal("Enqueue beyond MaxQueueDepth: want ErrQueueFull, got nil")
	}
	if errors.GetCode(err) != ErrQueueFull {
		t.Fatalf("error code = %v, want ErrQueueFull", errors.GetCode(err))
	}
}

func TestBackpressureReleasesDepthOnCompletion(t *testing.T) {
	clock := newFakeClock(time.Now())
	p := Ephemeral()
	p.MaxQueueDepth = 1
	q := newTestQueue(t, p, clock)
	q.Handle("t", func(context.Context, Job) error { return nil })

	if _, err := q.Enqueue(context.Background(), "t", nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.Enqueue(context.Background(), "t", nil); err == nil {
		t.Fatal("expected ErrQueueFull at depth 1")
	}

	q.processTick()
	q.wg.Wait()

	if _, err := q.Enqueue(context.Background(), "t", nil); err != nil {
		t.Fatalf("Enqueue after completion freed depth: %v", err)
	}
}

func TestRetryReschedulesWithBackoff(t *testing.T) {
	clock := newFakeClock(time.Now())
	p := Ephemeral()
	p.MaxAttempts = 5
	p.Backoff = func(int) time.Duration { return 10 * time.Second }
	q := newTestQueue(t, p, clock)

	var attempts int
	q.Handle("t", func(context.Context, Job) error {
		attempts++
		return fmt.Errorf("boom")
	})

	if _, err := q.Enqueue(context.Background(), "t", nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	q.processTick()
	q.wg.Wait()
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}

	// Backoff hasn't elapsed — the job must not run again yet.
	q.processTick()
	q.wg.Wait()
	if attempts != 1 {
		t.Fatalf("attempts = %d after a tick within the backoff window, want still 1", attempts)
	}

	clock.Advance(11 * time.Second)
	q.processTick()
	q.wg.Wait()
	if attempts != 2 {
		t.Fatalf("attempts = %d after the backoff window elapsed, want 2", attempts)
	}
}

func TestNoHandlerFailsJob(t *testing.T) {
	clock := newFakeClock(time.Now())
	p := Ephemeral()
	p.MaxAttempts = 1
	mem := events.NewMemoryEmitter()
	q := newTestQueue(t, p, clock, WithEmitter(events.NewEmitter(mem)))
	// Deliberately no Handle call for "t".

	if _, err := q.Enqueue(context.Background(), "t", nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	q.processTick()
	q.wg.Wait()

	found := false
	for _, r := range mem.Events() {
		if r.Name == EventJobFailed {
			found = true
		}
	}
	if !found {
		t.Fatal("expected runner:job_failed for a job with no registered handler")
	}
}

func TestDeadLetterRetainsJob(t *testing.T) {
	clock := newFakeClock(time.Now())
	p := Ephemeral()
	p.MaxAttempts = 2
	p.DeadLetter = true
	p.Backoff = func(int) time.Duration { return 0 }
	mem := events.NewMemoryEmitter()
	q := newTestQueue(t, p, clock, WithEmitter(events.NewEmitter(mem)))

	q.Handle("t", func(context.Context, Job) error { return fmt.Errorf("boom") })
	id, err := q.Enqueue(context.Background(), "t", nil)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	for i := 0; i < 2; i++ {
		q.processTick()
		q.wg.Wait()
	}

	dead, err := q.Dead()
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	if len(dead) != 1 || dead[0].ID != id {
		t.Fatalf("Dead() = %+v, want [%s]", dead, id)
	}

	found := false
	for _, r := range mem.Events() {
		if r.Name == EventJobDead {
			found = true
		}
	}
	if !found {
		t.Fatal("expected runner:job_dead event")
	}
}

func TestDropWithoutDeadLetter(t *testing.T) {
	clock := newFakeClock(time.Now())
	p := Ephemeral()
	p.MaxAttempts = 2
	p.DeadLetter = false
	p.Backoff = func(int) time.Duration { return 0 }
	mem := events.NewMemoryEmitter()
	q := newTestQueue(t, p, clock, WithEmitter(events.NewEmitter(mem)))

	q.Handle("t", func(context.Context, Job) error { return fmt.Errorf("boom") })
	if _, err := q.Enqueue(context.Background(), "t", nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	for i := 0; i < 2; i++ {
		q.processTick()
		q.wg.Wait()
	}

	dead, err := q.Dead()
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	if len(dead) != 0 {
		t.Fatalf("Dead() = %+v, want none — DeadLetter=false drops instead of retaining", dead)
	}

	var sawFailed, sawDead bool
	for _, r := range mem.Events() {
		switch r.Name {
		case EventJobFailed:
			sawFailed = true
		case EventJobDead:
			sawDead = true
		}
	}
	if !sawFailed {
		t.Error("expected runner:job_failed event")
	}
	if sawDead {
		t.Error("did not expect runner:job_dead event when DeadLetter=false")
	}
}

func TestProfileOverridesAffectBehavior(t *testing.T) {
	clock := newFakeClock(time.Now())
	p := Durable() // MaxAttempts == 0 (unlimited) by default
	p.MaxAttempts = 2
	p.Backoff = func(int) time.Duration { return 0 }
	store := newMemoryStore()
	q, err := New(p, WithStore(store), WithClock(clock.Now))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var attempts int
	q.Handle("t", func(context.Context, Job) error {
		attempts++
		return fmt.Errorf("boom")
	})
	id, err := q.Enqueue(context.Background(), "t", nil)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	for i := 0; i < 5; i++ {
		q.processTick()
		q.wg.Wait()
	}

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2 — the MaxAttempts override, not Durable's default of unlimited", attempts)
	}
	dead, err := q.Dead()
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	found := false
	for _, j := range dead {
		if j.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the job to be dead-lettered — Durable's DeadLetter=true still applies after overriding MaxAttempts")
	}
}

func TestWakeRescanCoalescesDueScan(t *testing.T) {
	clock := newFakeClock(time.Now())
	p := Ephemeral()
	p.ResumeOnWake = true
	mem := events.NewMemoryEmitter()
	q := newTestQueue(t, p, clock, WithEmitter(events.NewEmitter(mem)))
	q.tickInterval = time.Second

	var runs int32
	q.Handle("t", func(context.Context, Job) error {
		atomic.AddInt32(&runs, 1)
		return nil
	})

	// Establish the lastTick baseline.
	q.processTick()
	q.wg.Wait()

	if _, err := q.Enqueue(context.Background(), "t", nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Simulate the machine sleeping through many missed ticks: jump the
	// clock far past 2x the tick interval in one step, never calling
	// processTick in between.
	clock.Advance(50 * time.Second)
	q.processTick()
	q.wg.Wait()

	if got := atomic.LoadInt32(&runs); got != 1 {
		t.Fatalf("handler ran %d times across the gap, want exactly 1 — a coalesced due-scan, not a catch-up burst", got)
	}

	var wakeEvents int
	for _, r := range mem.Events() {
		if r.Name == EventWake {
			wakeEvents++
			payload, ok := r.Data.(WakePayload)
			if !ok {
				t.Fatalf("EventWake payload type = %T, want WakePayload", r.Data)
			}
			if payload.Gap < 40*time.Second {
				t.Fatalf("WakePayload.Gap = %v, want >= 40s", payload.Gap)
			}
		}
	}
	if wakeEvents != 1 {
		t.Fatalf("EventWake fired %d times, want exactly 1", wakeEvents)
	}
}

func TestWakeGapNotEmittedWithoutResumeOnWake(t *testing.T) {
	clock := newFakeClock(time.Now())
	p := Ephemeral()
	p.ResumeOnWake = false
	mem := events.NewMemoryEmitter()
	q := newTestQueue(t, p, clock, WithEmitter(events.NewEmitter(mem)))
	q.tickInterval = time.Second

	q.processTick()
	clock.Advance(50 * time.Second)
	q.processTick()
	q.wg.Wait()

	for _, r := range mem.Events() {
		if r.Name == EventWake {
			t.Fatal("EventWake fired despite ResumeOnWake == false")
		}
	}
}

// TestAtLeastOnceRedeliveryAfterCrash is the crash-mid-job scenario the WP
// calls out explicitly: a job left JobStateRunning by a previous, uncleanly
// terminated process must be redispatched (at least once), not lost and
// not left stuck forever.
func TestAtLeastOnceRedeliveryAfterCrash(t *testing.T) {
	store := newMemoryStore()
	now := time.Now()
	crashed := Job{
		ID:         "crashed-1",
		Type:       "t",
		Payload:    json.RawMessage(`{}`),
		State:      JobStateRunning, // left running by a simulated crash
		Attempts:   1,               // the attempt that was in flight when it crashed
		EnqueuedAt: now.Add(-time.Minute),
		NextRunAt:  now.Add(-time.Minute),
	}
	if err := store.Append(crashed); err != nil {
		t.Fatalf("Append: %v", err)
	}

	clock := newFakeClock(now)
	q, err := New(Durable(), WithStore(store), WithClock(clock.Now))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var gotAttempts int
	var handlerRan bool
	q.Handle("t", func(_ context.Context, job Job) error {
		gotAttempts = job.Attempts
		handlerRan = true
		return nil
	})

	if err := q.recoverOnce(); err != nil {
		t.Fatalf("recoverOnce: %v", err)
	}
	q.processTick()
	q.wg.Wait()

	if !handlerRan {
		t.Fatal("the crashed (JobStateRunning) job was not redispatched")
	}
	if gotAttempts != 2 {
		t.Fatalf("Attempts seen by handler = %d, want 2 (1 prior + 1 recovery dispatch)", gotAttempts)
	}

	due, err := store.Due(clock.Now(), 10)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	for _, j := range due {
		if j.ID == "crashed-1" {
			t.Fatalf("crashed job still reported due after completing: %+v", j)
		}
	}
}

func TestRequeueDeadJob(t *testing.T) {
	clock := newFakeClock(time.Now())
	p := Ephemeral()
	p.MaxAttempts = 1
	p.DeadLetter = true
	q := newTestQueue(t, p, clock)

	var attempts int
	q.Handle("t", func(context.Context, Job) error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("boom")
		}
		return nil
	})

	id, err := q.Enqueue(context.Background(), "t", nil)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	q.processTick()
	q.wg.Wait()

	dead, err := q.Dead()
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	if len(dead) != 1 {
		t.Fatalf("Dead() = %+v, want the job dead-lettered", dead)
	}

	if err := q.Requeue(id); err != nil {
		t.Fatalf("Requeue: %v", err)
	}

	dead, err = q.Dead()
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	if len(dead) != 0 {
		t.Fatalf("Dead() after Requeue = %+v, want none", dead)
	}

	q.processTick()
	q.wg.Wait()

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2 (requeue redispatched the job)", attempts)
	}
}

func TestRequeueUnknownJobErrors(t *testing.T) {
	q := newTestQueue(t, Ephemeral(), newFakeClock(time.Now()))
	if err := q.Requeue("does-not-exist"); err == nil {
		t.Fatal("Requeue on an unknown ID: want error, got nil")
	} else if errors.GetCode(err) != ErrJobNotFound {
		t.Fatalf("error code = %v, want ErrJobNotFound", errors.GetCode(err))
	}
}

// TestBoundedShutdownWithHungHandler proves Start returns promptly once
// ctx is cancelled, even while a handler is still "in flight" — as long as
// that handler respects ctx (the documented Handler contract). No
// goroutine leaks: Start waits for the handler goroutine to actually
// return before Start itself returns.
func TestBoundedShutdownWithHungHandler(t *testing.T) {
	clock := newFakeClock(time.Now())
	q := newTestQueue(t, Ephemeral(), clock)
	q.tickInterval = 5 * time.Millisecond

	handlerStarted := make(chan struct{})
	q.Handle("t", func(ctx context.Context, _ Job) error {
		close(handlerStarted)
		<-ctx.Done() // respects cancellation — the Handler contract
		return ctx.Err()
	})

	if _, err := q.Enqueue(context.Background(), "t", nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	startDone := make(chan error, 1)
	go func() { startDone <- q.Start(ctx) }()

	select {
	case <-handlerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never started")
	}

	cancelledAt := time.Now()
	cancel()

	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("Start returned an error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return within 2s of ctx cancellation — shutdown is not bounded")
	}
	if elapsed := time.Since(cancelledAt); elapsed > time.Second {
		t.Fatalf("Start took %v to return after cancel, want well under 1s", elapsed)
	}
}

func TestEnqueueAfterCloseErrors(t *testing.T) {
	clock := newFakeClock(time.Now())
	q := newTestQueue(t, Ephemeral(), clock)

	if err := q.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := q.Enqueue(context.Background(), "t", nil)
	if err == nil {
		t.Fatal("Enqueue after Close: want error, got nil")
	}
	if errors.GetCode(err) != ErrQueueClosed {
		t.Fatalf("error code = %v, want ErrQueueClosed", errors.GetCode(err))
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	clock := newFakeClock(time.Now())
	q := newTestQueue(t, Ephemeral(), clock)

	if err := q.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := q.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
