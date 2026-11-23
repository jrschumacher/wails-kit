package tasks

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
)

// Spec describes a task when it is added.
type Spec struct {
	// ID is the task's ID. Empty generates one ("task-1", "task-2", ...).
	ID string
	// Parent makes this task a part of another task in the list.
	Parent string
	// Subject and Kind are app-defined: what the task acts on, and what
	// sort of work it is.
	Subject string
	Kind    string
	// Words are the app's words for the work ("Exporting"). A retry starts
	// again from these.
	Words string
	// Stop is how the task can be stopped; StopNone if it can't.
	Stop StopMode
	// Retry is the retry policy; empty means RetryManual.
	Retry RetryPolicy
	// MaxAttempts caps the attempts of a RetryAuto task; 0 retries for as
	// long as it fails, every 30 s once the backoff is used up.
	MaxAttempts int
	// Timeout is the timeout on silence while running: 0 uses the service's,
	// NoTimeout turns it off.
	Timeout time.Duration
	// Queued adds the task as Queued instead of Running.
	Queued bool

	// OnStop is called when the person asks to stop the task (Service.Stop).
	// The app stops the work and reports Cancelled or Stopped. Without it,
	// Stop cancels the task's context.
	OnStop func(*Task)
	// OnPause and OnResume are called by Service.Pause and Service.Resume.
	// The app pauses or resumes the work and reports Paused or Running.
	OnPause  func(*Task)
	OnResume func(*Task)
	// OnRetry runs the work again, on Service.Retry or an automatic retry.
	// The task is already Running, with a fresh context. A task without it
	// can't be retried.
	OnRetry func(*Task)
}

// ServiceOption configures a Service.
type ServiceOption func(*Service)

// WithEmitter sets the emitter that tasks:changed goes out on. Give it
// events.WithHistory so ReplayAll sends a reloaded window the current list.
func WithEmitter(e *events.Emitter) ServiceOption {
	return func(s *Service) { s.emitter = e }
}

// WithClock sets the time source. Tests inject a fake.
func WithClock(c Clock) ServiceOption {
	return func(s *Service) {
		if c != nil {
			s.clock = c
		}
	}
}

// WithProgressInterval sets the shortest time between snapshots sent for
// progress alone (default 250 ms: four a second). Changes of state go out at
// once.
func WithProgressInterval(d time.Duration) ServiceOption {
	return func(s *Service) {
		if d >= 0 {
			s.progressEvery = d
		}
	}
}

// WithTimeout sets the default timeout on silence for running tasks
// (default 2 min). NoTimeout turns it off for every task that doesn't set
// its own.
func WithTimeout(d time.Duration) ServiceOption {
	return func(s *Service) {
		if d != 0 {
			s.timeout = d
		}
	}
}

// WithBackoff sets the waits before automatic retries; the last repeats.
// jitter is the random spread, as a fraction (0.2 is ±20%).
func WithBackoff(waits []time.Duration, jitter float64) ServiceOption {
	return func(s *Service) {
		if len(waits) > 0 {
			s.backoff = append([]time.Duration(nil), waits...)
		}
		if jitter >= 0 && jitter < 1 {
			s.jitter = jitter
		}
	}
}

// WithRandom sets the source of jitter: a function returning a value in
// [0, 1). Tests inject a fixed one.
func WithRandom(f func() float64) ServiceOption {
	return func(s *Service) {
		if f != nil {
			s.random = f
		}
	}
}

// WithWakeDetection sets how wake is detected from a jump in the clock:
// while any task is active, the service checks the wall clock every
// interval, and treats a gap longer than interval+threshold as a wake.
// Defaults: 5 s and 15 s. WithWakeDetection(0, 0) turns it off, for an app
// that calls Sleep and Wake itself on every platform.
func WithWakeDetection(interval, threshold time.Duration) ServiceOption {
	return func(s *Service) {
		if interval < 0 || threshold < 0 {
			return
		}
		s.wakeEvery = interval
		s.wakeThreshold = threshold
	}
}

// Service keeps the list of tasks and emits tasks:changed. It is safe for
// concurrent use. Its exported methods (Tasks, Stop, Pause, Resume, Retry,
// Dismiss) can be bound for the frontend.
type Service struct {
	emitter       *events.Emitter
	clock         Clock
	progressEvery time.Duration
	timeout       time.Duration
	backoff       []time.Duration
	jitter        float64
	random        func() float64
	wakeEvery     time.Duration
	wakeThreshold time.Duration

	baseCtx    context.Context
	baseCancel context.CancelFunc

	mu     sync.Mutex
	tasks  map[string]*Task
	order  []*Task
	nextID int
	closed bool
	asleep bool

	// Emission: one goroutine emits at a time; changes made meanwhile set
	// dirty, and the emitting goroutine sends them when it's done.
	emitting bool
	dirty    bool
	lastEmit time.Time
	flush    Timer
	flushGen uint64

	detector Timer
	lastTick time.Time
}

// NewService creates a task service.
func NewService(opts ...ServiceOption) *Service {
	s := &Service{
		clock:         realClock{},
		progressEvery: DefaultProgressInterval,
		timeout:       DefaultTimeout,
		backoff:       DefaultBackoff,
		jitter:        DefaultBackoffJitter,
		random:        rand.Float64,
		wakeEvery:     DefaultWakeInterval,
		wakeThreshold: DefaultWakeThreshold,
		tasks:         make(map[string]*Task),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.baseCtx, s.baseCancel = context.WithCancel(context.Background())
	return s
}

// Add adds a task to the list and emits a snapshot. The task starts Running,
// or Queued with Spec.Queued.
func (s *Service) Add(spec Spec) (*Task, error) {
	if !spec.Stop.valid() {
		return nil, errors.New(ErrInvalidSpec, fmt.Sprintf("unknown stop mode %q", spec.Stop), nil)
	}
	switch spec.Retry {
	case "":
		spec.Retry = RetryManual
	case RetryAuto, RetryManual, RetryUnsafe:
	default:
		return nil, errors.New(ErrInvalidSpec, fmt.Sprintf("unknown retry policy %q", spec.Retry), nil)
	}
	if spec.MaxAttempts < 0 {
		return nil, errors.New(ErrInvalidSpec, "max attempts is negative", nil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New(ErrInvalidState, "task service is closed", nil)
	}
	if spec.ID == "" {
		for {
			s.nextID++
			spec.ID = fmt.Sprintf("task-%d", s.nextID)
			if _, taken := s.tasks[spec.ID]; !taken {
				break
			}
		}
	} else if _, taken := s.tasks[spec.ID]; taken {
		return nil, errors.New(ErrDuplicateID, fmt.Sprintf("task %q is already in the list", spec.ID), nil)
	}
	if spec.Parent != "" {
		if _, ok := s.tasks[spec.Parent]; !ok {
			return nil, errors.New(ErrNotFound, fmt.Sprintf("parent task %q is not in the list", spec.Parent), nil)
		}
	}

	now := s.clock.Now()
	t := &Task{
		svc:  s,
		spec: spec,
		snap: Snapshot{
			ID:        spec.ID,
			Parent:    spec.Parent,
			Subject:   spec.Subject,
			Kind:      spec.Kind,
			State:     StateRunning,
			Words:     spec.Words,
			Stop:      spec.Stop,
			Retry:     spec.Retry,
			UpdatedAt: millis(now),
		},
		timeout: spec.Timeout,
	}
	t.ctx, t.cancel = context.WithCancel(s.baseCtx)
	if spec.Queued {
		t.snap.State = StateQueued
	} else {
		t.snap.StartedAt = millisPtr(now)
	}
	s.tasks[spec.ID] = t
	s.order = append(s.order, t)
	s.armSilenceLocked(t)
	s.armDetectorLocked()
	s.changedLocked(true)
	return t, nil
}

// Tasks returns a snapshot of every task in the list. A window that has just
// loaded can call it instead of waiting for the next tasks:changed.
func (s *Service) Tasks() []Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotsLocked()
}

// Get returns one task's snapshot.
func (s *Service) Get(id string) (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return Snapshot{}, false
	}
	return t.snap, true
}

// Stop asks to stop a task. A task waiting for an automatic retry is
// Cancelled at once. Otherwise the task's OnStop runs; without one, the
// task's context is cancelled and the work reports Cancelled or Stopped.
func (s *Service) Stop(id string) error {
	s.mu.Lock()
	t, err := s.activeLocked(id)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if t.snap.Stop == StopNone {
		s.mu.Unlock()
		return errors.New(ErrUnsupported, fmt.Sprintf("task %q can't be stopped", id), nil)
	}
	if t.retryPendingLocked() {
		s.finishLocked(t, StateCancelled, "", nil)
		s.changedLocked(true)
		s.mu.Unlock()
		return nil
	}
	onStop := t.spec.OnStop
	if onStop == nil {
		t.cancel()
	}
	s.mu.Unlock()
	if onStop != nil {
		onStop(t)
	}
	return nil
}

// Pause asks to pause a task through its OnPause.
func (s *Service) Pause(id string) error {
	return s.control(id, func(t *Task) func(*Task) { return t.spec.OnPause }, "paused")
}

// Resume asks to resume a paused task through its OnResume.
func (s *Service) Resume(id string) error {
	return s.control(id, func(t *Task) func(*Task) { return t.spec.OnResume }, "resumed")
}

func (s *Service) control(id string, pick func(*Task) func(*Task), verb string) error {
	s.mu.Lock()
	t, err := s.activeLocked(id)
	var fn func(*Task)
	if err == nil {
		fn = pick(t)
		if fn == nil {
			err = errors.New(ErrUnsupported, fmt.Sprintf("task %q can't be %s", id, verb), nil)
		}
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	fn(t)
	return nil
}

// Retry runs a task again now: a Failed task, or one waiting for an
// automatic retry ("Retry now"). The backoff carries on from where it was if
// this attempt fails too.
func (s *Service) Retry(id string) error {
	s.mu.Lock()
	t, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return errors.New(ErrNotFound, fmt.Sprintf("task %q is not in the list", id), nil)
	}
	if t.spec.OnRetry == nil {
		s.mu.Unlock()
		return errors.New(ErrUnsupported, fmt.Sprintf("task %q can't be retried", id), nil)
	}
	if t.snap.State != StateFailed && !t.retryPendingLocked() {
		s.mu.Unlock()
		return errors.New(ErrInvalidState, fmt.Sprintf("task %q is %s, not failed", id, t.snap.State), nil)
	}
	s.beginAttemptLocked(t)
	s.changedLocked(true)
	s.mu.Unlock()
	t.spec.OnRetry(t)
	return nil
}

// Dismiss removes a finished task, and its parts, from the list.
func (s *Service) Dismiss(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return errors.New(ErrNotFound, fmt.Sprintf("task %q is not in the list", id), nil)
	}
	family := s.familyLocked(t)
	for _, m := range family {
		if !m.snap.State.Finished() {
			return errors.New(ErrInvalidState, fmt.Sprintf("task %q is %s, not finished", m.snap.ID, m.snap.State), nil)
		}
	}
	s.removeLocked(family)
	s.changedLocked(true)
	return nil
}

// DismissFinished removes every finished top-level task whose parts have all
// finished too, and returns how many tasks it removed.
func (s *Service) DismissFinished() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	var remove []*Task
	for _, t := range s.order {
		if t.snap.Parent != "" {
			continue
		}
		family := s.familyLocked(t)
		done := true
		for _, m := range family {
			if !m.snap.State.Finished() {
				done = false
				break
			}
		}
		if done {
			remove = append(remove, family...)
		}
	}
	if len(remove) > 0 {
		s.removeLocked(remove)
		s.changedLocked(true)
	}
	return len(remove)
}

// Sleep stops every clock. Call it when the system is going to sleep (Wails
// reports this on Windows). Wake starts them again.
func (s *Service) Sleep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asleep = true
	for _, t := range s.order {
		stopTimer(&t.timer)
		stopTimer(&t.retryTimer)
	}
}

// Wake starts every clock again from zero: each running task's timeout on
// silence restarts, and a task waiting for an automatic retry drops its
// backoff and tries at once. Call it on wake (Wails reports it on Windows);
// elsewhere the service detects wake from a jump in the clock.
func (s *Service) Wake() {
	s.mu.Lock()
	retries := s.wakeLocked()
	s.mu.Unlock()
	for _, t := range retries {
		t.spec.OnRetry(t)
	}
}

func (s *Service) wakeLocked() []*Task {
	s.asleep = false
	var retries []*Task
	for _, t := range s.order {
		if t.snap.State.Finished() {
			continue
		}
		if t.snap.NextTry != nil {
			t.attempts = 0
			s.beginAttemptLocked(t)
			retries = append(retries, t)
			continue
		}
		s.armSilenceLocked(t)
	}
	if len(retries) > 0 {
		s.changedLocked(true)
	}
	return retries
}

// Close stops every timer, cancels every task's context, and sends any
// snapshot still held back by coalescing. Later reports are ignored.
func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	for _, t := range s.order {
		stopTimer(&t.timer)
		stopTimer(&t.retryTimer)
	}
	stopTimer(&s.detector)
	pending := s.flush != nil
	if pending {
		s.changedLocked(true)
	}
	s.mu.Unlock()
	s.baseCancel()
}

// --- internals (all called with s.mu held) ---

func (s *Service) activeLocked(id string) (*Task, error) {
	t, ok := s.tasks[id]
	if !ok {
		return nil, errors.New(ErrNotFound, fmt.Sprintf("task %q is not in the list", id), nil)
	}
	if t.snap.State.Finished() {
		return nil, errors.New(ErrInvalidState, fmt.Sprintf("task %q has finished", id), nil)
	}
	return t, nil
}

func (s *Service) snapshotsLocked() []Snapshot {
	out := make([]Snapshot, len(s.order))
	for i, t := range s.order {
		out[i] = t.snap
	}
	return out
}

// familyLocked returns t and every part under it.
func (s *Service) familyLocked(t *Task) []*Task {
	family := []*Task{t}
	for i := 0; i < len(family); i++ {
		for _, c := range s.order {
			if c.snap.Parent == family[i].snap.ID {
				family = append(family, c)
			}
		}
	}
	return family
}

func (s *Service) removeLocked(remove []*Task) {
	gone := make(map[*Task]bool, len(remove))
	for _, t := range remove {
		gone[t] = true
		delete(s.tasks, t.snap.ID)
		stopTimer(&t.timer)
		stopTimer(&t.retryTimer)
		t.removed = true
		t.cancel()
	}
	kept := s.order[:0]
	for _, t := range s.order {
		if !gone[t] {
			kept = append(kept, t)
		}
	}
	for i := len(kept); i < len(s.order); i++ {
		s.order[i] = nil
	}
	s.order = kept
}

// touchLocked records a change and restarts the timeout on silence.
func (s *Service) touchLocked(t *Task) {
	t.snap.UpdatedAt = millis(s.clock.Now())
	s.armSilenceLocked(t)
}

func (s *Service) effectiveTimeout(t *Task) time.Duration {
	d := t.timeout
	if d == 0 {
		d = s.timeout
	}
	return d
}

// armSilenceLocked (re)starts the task's timeout on silence, which runs only
// while it is Running, or Waiting on its own work (not on a retry).
func (s *Service) armSilenceLocked(t *Task) {
	stopTimer(&t.timer)
	t.gen++
	if s.closed || s.asleep || t.removed {
		return
	}
	if t.snap.State != StateRunning && (t.snap.State != StateWaiting || t.snap.NextTry != nil) {
		return
	}
	d := s.effectiveTimeout(t)
	if d <= 0 {
		return
	}
	gen := t.gen
	t.timer = s.clock.AfterFunc(d, func() { s.onTimeout(t, gen, d) })
}

func (s *Service) onTimeout(t *Task, gen uint64, d time.Duration) {
	s.mu.Lock()
	if t.gen != gen || t.removed || s.closed || t.snap.State.Finished() {
		s.mu.Unlock()
		return
	}
	t.timer = nil
	err := errors.New(errors.ErrTimeout, fmt.Sprintf("task %q made no progress for %s", t.snap.ID, d), nil)
	s.failLocked(t, err, "Timed out after "+durationWords(d))
	s.mu.Unlock()
}

// failLocked fails the current attempt. An automatic retry policy waits
// with a backoff instead, if the task can be retried.
func (s *Service) failLocked(t *Task, err error, words string) {
	info := errorInfo(err)
	t.attempts++
	if t.spec.Retry == RetryAuto && t.spec.OnRetry != nil &&
		(t.spec.MaxAttempts == 0 || t.attempts < t.spec.MaxAttempts) {
		wait := s.backoffFor(t.attempts)
		now := s.clock.Now()
		stopTimer(&t.timer)
		t.gen++
		t.cancel()
		t.snap.State = StateWaiting
		t.snap.Error = info
		t.snap.Words = words
		t.snap.NextTry = millisPtr(now.Add(wait))
		t.snap.UpdatedAt = millis(now)
		gen := t.gen
		stopTimer(&t.retryTimer)
		if !s.asleep && !s.closed {
			t.retryTimer = s.clock.AfterFunc(wait, func() { s.autoRetry(t, gen) })
		}
		s.changedLocked(true)
		return
	}
	s.finishLocked(t, StateFailed, words, info)
	s.changedLocked(true)
}

func (s *Service) autoRetry(t *Task, gen uint64) {
	s.mu.Lock()
	if t.gen != gen || t.removed || s.closed || !t.retryPendingLocked() {
		s.mu.Unlock()
		return
	}
	t.retryTimer = nil
	s.beginAttemptLocked(t)
	s.changedLocked(true)
	s.mu.Unlock()
	t.spec.OnRetry(t)
}

// beginAttemptLocked starts a fresh attempt: Running, a new context, the
// error and next try cleared, and the words back to the task's own.
func (s *Service) beginAttemptLocked(t *Task) {
	stopTimer(&t.retryTimer)
	t.cancel()
	t.ctx, t.cancel = context.WithCancel(s.baseCtx)
	t.snap.State = StateRunning
	t.snap.Error = nil
	t.snap.NextTry = nil
	t.snap.Words = t.spec.Words
	if t.snap.StartedAt == nil {
		t.snap.StartedAt = millisPtr(s.clock.Now())
	}
	s.touchLocked(t)
	s.armDetectorLocked()
}

func (s *Service) finishLocked(t *Task, state State, words string, info *ErrorInfo) {
	stopTimer(&t.timer)
	stopTimer(&t.retryTimer)
	t.gen++
	t.cancel()
	t.snap.State = state
	t.snap.Words = words
	t.snap.Error = info
	t.snap.NextTry = nil
	t.snap.UpdatedAt = millis(s.clock.Now())
	if state == StateDone {
		t.attempts = 0
	}
}

func (s *Service) backoffFor(attempt int) time.Duration {
	i := attempt - 1
	if i < 0 {
		i = 0
	}
	if i >= len(s.backoff) {
		i = len(s.backoff) - 1
	}
	base := s.backoff[i]
	f := 1 + s.jitter*(2*s.random()-1)
	return time.Duration(float64(base) * f)
}

func errorInfo(err error) *ErrorInfo {
	if err == nil {
		return &ErrorInfo{Code: errors.ErrInternal, UserMsg: errors.GetUserMessage(nil)}
	}
	return &ErrorInfo{
		Code:    errors.GetCode(err),
		UserMsg: errors.GetUserMessage(err),
		Message: err.Error(),
	}
}

// changedLocked sends a snapshot. A change of state (urgent) goes out at
// once; progress alone goes out at most once per progress interval, and the
// last one always goes out.
func (s *Service) changedLocked(urgent bool) {
	if s.emitter == nil {
		return
	}
	if !urgent {
		if s.flush != nil || s.closed {
			return
		}
		wait := s.progressEvery - s.clock.Now().Sub(s.lastEmit)
		if wait > 0 || s.emitting {
			if wait <= 0 {
				wait = s.progressEvery
			}
			s.flushGen++
			gen := s.flushGen
			s.flush = s.clock.AfterFunc(wait, func() { s.onFlush(gen) })
			return
		}
	}
	s.dirty = true
	if s.emitting {
		return
	}
	s.emitting = true
	for s.dirty {
		s.dirty = false
		stopTimer(&s.flush)
		payload := ChangedPayload{Tasks: s.snapshotsLocked()}
		s.lastEmit = s.clock.Now()
		s.mu.Unlock()
		s.emitter.Emit(EventChanged, payload)
		s.mu.Lock()
	}
	s.emitting = false
}

func (s *Service) onFlush(gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.flush == nil || gen != s.flushGen {
		return
	}
	s.flush = nil
	s.changedLocked(true)
}

// armDetectorLocked starts the wake detector while any task is active.
func (s *Service) armDetectorLocked() {
	if s.wakeEvery <= 0 || s.detector != nil || s.closed {
		return
	}
	s.lastTick = s.clock.Now()
	s.detector = s.clock.AfterFunc(s.wakeEvery, s.onTick)
}

func (s *Service) onTick() {
	s.mu.Lock()
	if s.closed || s.detector == nil {
		s.mu.Unlock()
		return
	}
	s.detector = nil
	now := s.clock.Now()
	// Round(0) drops the monotonic reading: the monotonic clock may not
	// count time asleep, the wall clock does.
	gap := now.Round(0).Sub(s.lastTick.Round(0))
	var retries []*Task
	if gap > s.wakeEvery+s.wakeThreshold {
		retries = s.wakeLocked()
	}
	for _, t := range s.order {
		if !t.snap.State.Finished() {
			s.armDetectorLocked()
			break
		}
	}
	s.mu.Unlock()
	for _, t := range retries {
		t.spec.OnRetry(t)
	}
}

func stopTimer(t *Timer) {
	if *t != nil {
		(*t).Stop()
		*t = nil
	}
}
