package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
)

// defaultTickInterval is how often Start scans for due jobs when the
// caller hasn't configured anything else. There is no WithTickInterval in
// the authoritative API (see docs/v2-roadmap.md WP-40) — it is fixed
// rather than exposed as an Option, since exposing it invites tuning a
// desktop app's queue like a server's, which is exactly what this package
// deliberately is not.
const defaultTickInterval = time.Second

// Queue is a durable job queue plus a worker that ticks. See the package
// doc for the overall model.
type Queue struct {
	profile Profile
	store   Store
	emitter *events.Emitter
	clock   func() time.Time
	workers int

	tickInterval time.Duration

	mu          sync.Mutex
	handlers    map[string]Handler
	closed      bool
	depth       int
	idempotency map[string]string // idempotency key -> job ID, pending/running only
	inFlight    map[string]struct{}
	deadJobs    map[string]Job // fallback registry when Store isn't a Lister
	lastTick    time.Time
	recovered   bool
	runCtx      context.Context

	wg sync.WaitGroup
}

// Option configures a Queue at construction.
type Option func(*Queue)

// WithStore sets the persistence backend. Required when
// Profile.Persistence == PersistStore; New returns ErrStoreRequired
// otherwise. Ignored (a built-in in-memory Store is used) when
// Persistence == PersistNone and WithStore was not supplied.
func WithStore(s Store) Option {
	return func(q *Queue) { q.store = s }
}

// WithEmitter sets the events.Emitter used for EventJobDone/Failed/Dead/Wake.
// A nil emitter (the default) means events are silently dropped, matching
// AD-9's "optional *events.Emitter (nil -> drop)" convention.
func WithEmitter(e *events.Emitter) Option {
	return func(q *Queue) { q.emitter = e }
}

// WithWorkers sets how many jobs may be dispatched concurrently. Default 1
// — this is desktop job-queue infrastructure, not a server worker pool.
func WithWorkers(n int) Option {
	return func(q *Queue) {
		if n > 0 {
			q.workers = n
		}
	}
}

// WithClock overrides the queue's notion of "now", for tests. Production
// code never needs this — the default is time.Now.
func WithClock(c func() time.Time) Option {
	return func(q *Queue) {
		if c != nil {
			q.clock = c
		}
	}
}

// New constructs a Queue from a Profile. See Ephemeral/Durable/BestEffort.
func New(p Profile, opts ...Option) (*Queue, error) {
	if p.Backoff == nil {
		p.Backoff = DefaultBackoff
	}

	q := &Queue{
		profile:      p,
		clock:        time.Now,
		workers:      1,
		tickInterval: defaultTickInterval,
		handlers:     make(map[string]Handler),
		idempotency:  make(map[string]string),
		inFlight:     make(map[string]struct{}),
		deadJobs:     make(map[string]Job),
		// runCtx defaults to context.Background() so a test (or any
		// caller) that dispatches jobs via processTick without going
		// through Start still gives handlers a non-nil context. Start
		// overwrites this with its own ctx.
		runCtx: context.Background(),
	}
	for _, opt := range opts {
		opt(q)
	}

	switch p.Persistence {
	case PersistStore:
		if q.store == nil {
			return nil, errors.New(ErrStoreRequired, "runner.New: Profile.Persistence == PersistStore requires WithStore", nil)
		}
	default:
		if q.store == nil {
			q.store = newMemoryStore()
		}
	}

	// Bootstrap depth/idempotency/dead-job bookkeeping from the Store here,
	// not in Start. Enqueue already increments q.depth (and the
	// idempotency map) for every job it appends; if this same bootstrap
	// also ran in Start (as it used to), any job Enqueued between New and
	// Start would be counted twice — once by Enqueue itself, once by the
	// Store-derived recovery scan finding the very same job it had just
	// persisted. Running it here, before the caller can possibly have
	// enqueued anything, means every subsequent Enqueue is the only thing
	// that ever increments depth for that job. recoverOnce is idempotent
	// (guarded by q.recovered) so Start's own call remains harmless.
	if err := q.recoverOnce(); err != nil {
		return nil, fmt.Errorf("runner: recover: %w", err)
	}

	return q, nil
}

// Handle registers h as the handler for jobType. Calling Handle again for
// the same jobType replaces the previous handler. Register all handlers
// before calling Start.
func (q *Queue) Handle(jobType string, h Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[jobType] = h
}

func (q *Queue) handlerFor(jobType string) (Handler, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	h, ok := q.handlers[jobType]
	return h, ok
}

// EnqueueOption configures a single Enqueue call.
type EnqueueOption func(*enqueueConfig)

type enqueueConfig struct {
	idempotencyKey string
	delay          time.Duration
}

// WithIdempotencyKey deduplicates Enqueue calls: if a job with the same key
// is currently pending or running, Enqueue returns that job's existing ID
// instead of creating a new one. Once that job reaches a terminal state
// (done/failed/dead) the key is released and a later Enqueue with the same
// key creates a fresh job.
func WithIdempotencyKey(k string) EnqueueOption {
	return func(c *enqueueConfig) { c.idempotencyKey = k }
}

// WithDelay makes the job due d after it is enqueued, instead of
// immediately.
func WithDelay(d time.Duration) EnqueueOption {
	return func(c *enqueueConfig) { c.delay = d }
}

// Enqueue adds a new job of jobType with payload (marshaled to JSON) and
// returns its ID. It returns ErrQueueFull if Profile.MaxQueueDepth is set
// and would be exceeded, and ErrQueueClosed if the queue has been Closed.
func (q *Queue) Enqueue(_ context.Context, jobType string, payload any, opts ...EnqueueOption) (string, error) {
	var cfg enqueueConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	raw, err := marshalPayload(payload)
	if err != nil {
		return "", fmt.Errorf("runner: marshal payload: %w", err)
	}

	now := q.clock()

	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return "", errors.New(ErrQueueClosed, "runner.Enqueue: called after Close", nil)
	}
	if cfg.idempotencyKey != "" {
		if existing, ok := q.idempotency[cfg.idempotencyKey]; ok {
			q.mu.Unlock()
			return existing, nil
		}
	}
	if q.profile.MaxQueueDepth > 0 && q.depth >= q.profile.MaxQueueDepth {
		q.mu.Unlock()
		return "", errors.New(ErrQueueFull, "runner.Enqueue: queue depth would exceed MaxQueueDepth", nil)
	}

	job := Job{
		ID:             newJobID(),
		Type:           jobType,
		Payload:        raw,
		State:          JobStatePending,
		IdempotencyKey: cfg.idempotencyKey,
		EnqueuedAt:     now,
		NextRunAt:      now.Add(cfg.delay),
	}

	// Append while still holding q.mu is deliberate: it keeps the
	// depth/idempotency bookkeeping and the Store write atomic with
	// respect to concurrent Enqueue calls, at the cost of serializing
	// Store.Append behind the queue's single mutex. Fine for a desktop
	// queue's Append rate; not a fit for a high-throughput server queue.
	if err := q.store.Append(job); err != nil {
		q.mu.Unlock()
		return "", fmt.Errorf("runner: append job: %w", err)
	}
	q.depth++
	if cfg.idempotencyKey != "" {
		q.idempotency[cfg.idempotencyKey] = job.ID
	}
	q.mu.Unlock()

	return job.ID, nil
}

func marshalPayload(payload any) (json.RawMessage, error) {
	if raw, ok := payload.(json.RawMessage); ok {
		return raw, nil
	}
	if raw, ok := payload.([]byte); ok {
		return json.RawMessage(raw), nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// Dead returns every job currently in JobStateDead. When the Store also
// implements Lister, this reflects every dead-lettered job the Store
// knows about (including ones dead-lettered by a previous process
// instance); otherwise it only reflects jobs dead-lettered since this
// Queue was constructed — see Lister's doc comment.
func (q *Queue) Dead() ([]Job, error) {
	if lister, ok := q.store.(Lister); ok {
		return lister.List(JobStateDead)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Job, 0, len(q.deadJobs))
	for _, j := range q.deadJobs {
		out = append(out, j)
	}
	sortJobs(out)
	return out, nil
}

// Requeue moves a dead-lettered job back to JobStatePending, resetting
// Attempts to 0, so it is picked up on the next due-scan. It returns
// ErrJobNotFound if id is not currently dead.
func (q *Queue) Requeue(id string) error {
	dead, err := q.Dead()
	if err != nil {
		return err
	}
	var job Job
	var found bool
	for _, j := range dead {
		if j.ID == id {
			job, found = j, true
			break
		}
	}
	if !found {
		return errors.New(ErrJobNotFound, "runner.Requeue: job "+id+" is not dead-lettered", nil)
	}

	job.State = JobStatePending
	job.Attempts = 0
	job.LastError = ""
	job.NextRunAt = q.clock()

	if err := q.store.Update(job); err != nil {
		return fmt.Errorf("runner: requeue job: %w", err)
	}

	q.mu.Lock()
	delete(q.deadJobs, id)
	q.depth++
	q.mu.Unlock()

	return nil
}

// Discard permanently removes a dead-lettered job identified by id — the
// only way a dead job's Store record can ever go away, since Sweep never
// touches JobStateDead (DeadLetter/Retention govern separate lifetimes;
// see Sweep's doc comment) and Requeue moves a job back to pending rather
// than deleting it. It returns ErrJobNotFound if id is not currently dead,
// and ErrDiscardUnsupported if the Store doesn't implement Deleter (in
// which case nothing is removed — Discard never pretends success while
// leaving the record behind).
func (q *Queue) Discard(id string) error {
	dead, err := q.Dead()
	if err != nil {
		return err
	}
	found := false
	for _, j := range dead {
		if j.ID == id {
			found = true
			break
		}
	}
	if !found {
		return errors.New(ErrJobNotFound, "runner.Discard: job "+id+" is not dead-lettered", nil)
	}

	deleter, ok := q.store.(Deleter)
	if !ok {
		return errors.New(ErrDiscardUnsupported, "runner.Discard: Store does not implement Deleter", nil)
	}
	if err := deleter.Delete(id); err != nil {
		return fmt.Errorf("runner: discard job: %w", err)
	}

	q.mu.Lock()
	delete(q.deadJobs, id)
	q.mu.Unlock()

	return nil
}

// Start runs the tick loop until ctx is done. On each tick it scans the
// Store for due jobs (including any left JobStateRunning by an unclean
// shutdown — see Store.Due) and dispatches up to Profile-implied
// concurrency (WithWorkers) of them. Start blocks until ctx.Done() fires
// and every dispatched handler goroutine has returned — a Handler that
// ignores ctx and never returns will block Start's return; see Handler's
// doc comment. Start is not safe to call concurrently with itself on the
// same Queue.
func (q *Queue) Start(ctx context.Context) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return errors.New(ErrQueueClosed, "runner.Start: called after Close", nil)
	}
	q.runCtx = ctx
	q.mu.Unlock()

	// recoverOnce already ran in New — this call is a no-op guarded by
	// q.recovered, kept here only so a hand-built Queue that skipped New's
	// error path (impossible via the public API, but cheap insurance) is
	// still bootstrapped before its first tick.
	if err := q.recoverOnce(); err != nil {
		return fmt.Errorf("runner: recover: %w", err)
	}

	ticker := time.NewTicker(q.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			q.wg.Wait()
			return nil
		case <-ticker.C:
			q.processTick()
		}
	}
}

// recoverOnce bootstraps depth/idempotency/dead-job bookkeeping from the
// Store, when it implements Lister. It runs at most once per Queue
// (idempotent to call again — see New, which calls this before returning
// the Queue to the caller, and Start, whose own call is then a no-op). It
// never dispatches anything — orphaned JobStateRunning jobs are picked up
// by the normal Due-based scan on the first tick, which is what actually
// performs crash recovery (see Store.Due's doc comment).
func (q *Queue) recoverOnce() error {
	q.mu.Lock()
	if q.recovered {
		q.mu.Unlock()
		return nil
	}
	q.recovered = true
	q.mu.Unlock()

	lister, ok := q.store.(Lister)
	if !ok {
		return nil
	}

	pending, err := lister.List(JobStatePending)
	if err != nil {
		return err
	}
	running, err := lister.List(JobStateRunning)
	if err != nil {
		return err
	}
	dead, err := lister.List(JobStateDead)
	if err != nil {
		return err
	}

	q.mu.Lock()
	for _, j := range append(pending, running...) {
		q.depth++
		if j.IdempotencyKey != "" {
			q.idempotency[j.IdempotencyKey] = j.ID
		}
	}
	for _, j := range dead {
		q.deadJobs[j.ID] = j
	}
	q.mu.Unlock()
	return nil
}

// processTick is one tick's worth of work: wake-gap detection, a due-scan,
// and a Sweep pass. It is deliberately parameter-free (reads q.clock()
// itself) so Start's real ticker and a test can call the exact same code
// path — tests inject a fake q.clock and call processTick directly,
// advancing the fake clock between calls, rather than waiting on real
// timers (see runner/AGENTS.md Testing).
func (q *Queue) processTick() {
	// Round(0) strips any monotonic clock reading now (q.clock defaults to
	// time.Now, which always attaches one). This matters because Go
	// compares two Times that both carry a monotonic reading using that
	// reading alone, ignoring the wall clock — and mach_absolute_time
	// (Darwin) / CLOCK_MONOTONIC (Linux) both freeze while the machine is
	// asleep. Without stripping it, `now.Sub(prev)` below would silently
	// report ~0 across a real sleep/wake cycle instead of the actual wall
	// gap, since both prev (q.lastTick, itself derived from q.clock) and
	// now would carry monotonic readings that never diverged. Stripping
	// either operand is sufficient — Sub falls back to wall-clock
	// comparison when either side lacks a monotonic reading — but
	// stripping at the point of capture keeps every downstream use of now
	// (and its storage into q.lastTick) consistently wall-clock-only.
	now := q.clock().Round(0)

	q.mu.Lock()
	prev := q.lastTick
	q.lastTick = now
	resumeOnWake := q.profile.ResumeOnWake
	closed := q.closed
	q.mu.Unlock()

	if closed {
		return
	}

	// Wake-gap detection is structural, not a catch-up loop: whether the
	// gap is one missed tick or a thousand, this call still performs
	// exactly one due-scan below — never one scan per missed interval.
	// That is what "coalesce missed ticks rather than firing a burst"
	// means in practice; there is no code path that could fire a burst.
	if !prev.IsZero() && resumeOnWake {
		if gap := now.Sub(prev); gap > 2*q.tickInterval {
			q.emit(EventWake, WakePayload{Gap: gap})
		}
	}

	q.scanDue(now)

	// Best-effort: a Sweep error just means stale terminal records linger
	// an extra tick or two; not worth failing the tick over.
	_ = q.store.Sweep(q.profile.Retention, now)
}

func (q *Queue) scanDue(now time.Time) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	avail := q.workers - len(q.inFlight)
	ctx := q.runCtx
	q.mu.Unlock()
	if avail <= 0 {
		return
	}

	// Due(now, avail) would cap the result at avail *before* this loop gets
	// a chance to skip jobs already tracked in q.inFlight. Due returns
	// in-flight running jobs alongside due pending ones (that's the crash-
	// recovery contract — see Store.Due's doc comment), and a running job
	// always sorts earliest (its NextRunAt predates any freshly-enqueued
	// pending job). With workers > 1, a limit of avail could come back
	// entirely full of jobs this process already has in flight, starving a
	// genuinely due pending job even though a worker is free — confirmed
	// with 2 workers, one long-running job, and a due pending job that
	// never dispatched across 5 ticks. Query unbounded (limit <= 0 means
	// "no limit" per the Store.Due contract) and let the loop below apply
	// the real dispatch cap after filtering out in-flight jobs.
	jobs, err := q.store.Due(now, 0)
	if err != nil {
		return
	}

	dispatched := 0
	for _, job := range jobs {
		if dispatched >= avail {
			break
		}
		q.mu.Lock()
		if _, busy := q.inFlight[job.ID]; busy {
			q.mu.Unlock()
			continue
		}
		if len(q.inFlight) >= q.workers {
			q.mu.Unlock()
			break
		}
		q.inFlight[job.ID] = struct{}{}
		q.mu.Unlock()

		q.wg.Add(1)
		go q.dispatch(ctx, job)
		dispatched++
	}
}

// dispatch runs one job's handler. job.State == JobStateRunning on entry
// means this job was left running by a previous, uncleanly-terminated
// process (Store.Due surfaced it) rather than being newly picked up by
// this Queue instance — see Store.Due's doc comment on crash recovery.
func (q *Queue) dispatch(ctx context.Context, job Job) {
	defer q.wg.Done()
	defer func() {
		q.mu.Lock()
		delete(q.inFlight, job.ID)
		q.mu.Unlock()
	}()

	recovered := job.State == JobStateRunning
	job.State = JobStateRunning
	job.Attempts++
	if recovered {
		job.LastError = "recovered after unclean shutdown"
	}
	// This Update is the state-transition that makes at-least-once
	// correct: it commits BEFORE the handler runs. If the process crashes
	// after this write but before the handler returns (or before the
	// matching Update below commits), the job is left JobStateRunning on
	// disk and Store.Due will surface it again on the next process's
	// first scan — the job runs again (at least once), never disappears
	// (never lost) or gets forgotten by the store (never silently
	// duplicated in the store itself, since Update replaces in place by
	// ID). Handlers that cannot tolerate the resulting re-run must use
	// their own idempotency check keyed on job.IdempotencyKey.
	if err := q.store.Update(job); err != nil {
		return
	}

	handler, ok := q.handlerFor(job.Type)
	if !ok {
		q.failJob(job, errors.New(ErrNoHandler, "runner: no handler registered for job type "+job.Type, nil))
		return
	}

	if err := handler(ctx, job); err != nil {
		q.failJob(job, err)
		return
	}
	q.completeJob(job)
}

func (q *Queue) completeJob(job Job) {
	job.State = JobStateDone
	job.LastError = ""
	// NextRunAt is repurposed as the completion timestamp for terminal
	// jobs — see Job.NextRunAt's doc comment.
	job.NextRunAt = q.clock()
	if err := q.store.Update(job); err != nil {
		return
	}
	q.releaseTerminal(job)
	q.emit(EventJobDone, JobEventPayload{Job: job})
}

func (q *Queue) failJob(job Job, cause error) {
	job.LastError = cause.Error()

	maxAttempts := q.profile.MaxAttempts
	terminal := maxAttempts > 0 && job.Attempts >= maxAttempts

	if !terminal {
		job.State = JobStatePending
		backoff := q.profile.Backoff
		if backoff == nil {
			backoff = DefaultBackoff
		}
		job.NextRunAt = q.clock().Add(backoff(job.Attempts))
		_ = q.store.Update(job)
		return
	}

	job.NextRunAt = q.clock() // repurposed as terminal timestamp
	if q.profile.DeadLetter {
		job.State = JobStateDead
	} else {
		job.State = JobStateFailed
	}
	if err := q.store.Update(job); err != nil {
		return
	}
	q.releaseTerminal(job)

	if job.State == JobStateDead {
		q.mu.Lock()
		q.deadJobs[job.ID] = job
		q.mu.Unlock()
		q.emit(EventJobDead, JobEventPayload{Job: job})
		return
	}
	q.emit(EventJobFailed, JobEventPayload{Job: job})
}

// releaseTerminal updates in-memory bookkeeping (depth, idempotency-key
// dedupe map) for a job that just reached a terminal state. It must never
// be called while q.mu is held by the caller — it takes the lock itself.
func (q *Queue) releaseTerminal(job Job) {
	q.mu.Lock()
	if q.depth > 0 {
		q.depth--
	}
	if job.IdempotencyKey != "" && q.idempotency[job.IdempotencyKey] == job.ID {
		delete(q.idempotency, job.IdempotencyKey)
	}
	q.mu.Unlock()
}

// emit sends an event if an emitter is configured. Always called without
// q.mu held (see the "never emit while holding a lock" invariant repeated
// throughout this package's callers) — four other kit packages have shipped
// or narrowly avoided that exact deadlock.
func (q *Queue) emit(name string, payload any) {
	q.mu.Lock()
	e := q.emitter
	q.mu.Unlock()
	if e == nil {
		return
	}
	e.Emit(name, payload)
}

// Close stops accepting new dispatches, waits for any in-flight handlers
// to finish (bounded by those handlers respecting the context Start gave
// them — see Handler's doc comment), and closes the underlying Store.
// Close is safe to call more than once; only the first call does work.
// Call Close after Start(ctx) has returned (i.e., after cancelling ctx) —
// calling it concurrently with a still-running Start can race Store.Close
// against Start's own Store calls.
func (q *Queue) Close() error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil
	}
	q.closed = true
	q.mu.Unlock()

	q.wg.Wait()
	return q.store.Close()
}
