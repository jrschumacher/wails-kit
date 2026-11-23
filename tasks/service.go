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
	// WaitingWords replace the words when the service moves the task to
	// Waiting after a quiet spell ("Waiting for GitHub…"). Empty keeps the
	// current words.
	WaitingWords string
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
	// Stop cancels the attempt's context (and cancels a queued task at once).
	OnStop func(*Task)
	// OnPause and OnResume are called by Service.Pause and Service.Resume.
	// The app pauses or resumes the work and reports Paused or Running.
	OnPause  func(*Task)
	OnResume func(*Task)
	// OnRetry runs the work again, on Service.Retry, an automatic retry, or
	// a retry at once on wake. It gets a new handle for the new attempt,
	// already Running with a fresh context. It runs synchronously (in the
	// caller of Retry or Wake, or on a timer), so it must start the work
	// (in a goroutine) and return quickly. A task without it can't be
	// retried.
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

// WithTimeoutWords sets the words of a task that timed out, given the
// timeout. The default is English: "Timed out after 2 min".
func WithTimeoutWords(f func(time.Duration) string) ServiceOption {
	return func(s *Service) {
		if f != nil {
			s.timeoutWords = f
		}
	}
}

// WithWaitingAfter sets how long a running task may go without a report
// before the service moves it to Waiting (default 30 s). NoTimeout turns the
// switch off.
func WithWaitingAfter(d time.Duration) ServiceOption {
	return func(s *Service) {
		if d != 0 {
			s.waitingAfter = d
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
// concurrent use. Bind Bindings() for the frontend, not the Service: the
// Service also has Add, Sleep, Wake and Close, which are the app's.
type Service struct {
	emitter       *events.Emitter
	clock         Clock
	progressEvery time.Duration
	timeout       time.Duration
	timeoutWords  func(time.Duration) string
	waitingAfter  time.Duration
	backoff       []time.Duration
	jitter        float64
	random        func() float64
	wakeEvery     time.Duration
	wakeThreshold time.Duration

	baseCtx    context.Context
	baseCancel context.CancelFunc

	mu     sync.Mutex
	tasks  map[string]*entry
	order  []*entry
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

// entry is one task in the list. Handles (*Task) point at it, one per
// attempt.
type entry struct {
	spec Spec
	snap Snapshot

	timeout     time.Duration // the task's current timeout on silence
	timer       Timer         // timeout on silence
	quiet       Timer         // the switch to Waiting
	retryTimer  Timer
	gen         uint64 // invalidates timer and quiet
	retryGen    uint64 // invalidates retryTimer
	attempt     uint64 // the current attempt; handles of others are stale
	attempts    int    // failed attempts, for MaxAttempts
	backoffStep int    // position in the backoff; wake resets it
	autoWaiting bool   // Waiting because of a quiet spell, not the app
	removed     bool
	current     *Task
}

// NewService creates a task service.
func NewService(opts ...ServiceOption) *Service {
	s := &Service{
		clock:         realClock{},
		progressEvery: DefaultProgressInterval,
		timeout:       DefaultTimeout,
		timeoutWords:  func(d time.Duration) string { return "Timed out after " + durationWords(d) },
		waitingAfter:  DefaultWaitingAfter,
		backoff:       DefaultBackoff,
		jitter:        DefaultBackoffJitter,
		random:        rand.Float64,
		wakeEvery:     DefaultWakeInterval,
		wakeThreshold: DefaultWakeThreshold,
		tasks:         make(map[string]*entry),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.baseCtx, s.baseCancel = context.WithCancel(context.Background())
	return s
}

// Add adds a task to the list and emits a snapshot. The task starts Running,
// or Queued with Spec.Queued. The returned handle reports the first attempt.
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
	e := &entry{
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
	if spec.Queued {
		e.snap.State = StateQueued
	} else {
		e.snap.StartedAt = millisPtr(now)
	}
	t := s.newHandleLocked(e)
	s.tasks[spec.ID] = e
	s.order = append(s.order, e)
	s.armClocksLocked(e)
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
	e, ok := s.tasks[id]
	if !ok {
		return Snapshot{}, false
	}
	return e.snap, true
}

// Stop asks to stop a task. A task waiting for an automatic retry finishes
// at once: Stopped if its stop mode is stop or stop-asks, else Cancelled.
// A queued task without an OnStop is Cancelled at once. Otherwise the task's
// OnStop runs; without one, the attempt's context is cancelled and the work
// reports Cancelled or Stopped.
func (s *Service) Stop(id string) error {
	s.mu.Lock()
	e, err := s.activeLocked(id)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if e.snap.Stop == StopNone {
		s.mu.Unlock()
		return errors.New(ErrUnsupported, fmt.Sprintf("task %q can't be stopped", id), nil)
	}
	if e.retryPending() {
		state := StateCancelled
		if e.snap.Stop == StopStop || e.snap.Stop == StopStopAsks {
			state = StateStopped
		}
		s.finishLocked(e, state, "", nil)
		s.changedLocked(true)
		s.mu.Unlock()
		return nil
	}
	onStop := e.spec.OnStop
	if onStop == nil && e.snap.State == StateQueued {
		s.finishLocked(e, StateCancelled, "", nil)
		s.changedLocked(true)
		s.mu.Unlock()
		return nil
	}
	h := e.current
	if onStop == nil {
		h.cancel()
	}
	s.mu.Unlock()
	if onStop != nil {
		onStop(h)
	}
	return nil
}

// Pause asks to pause a running or waiting task through its OnPause.
func (s *Service) Pause(id string) error {
	return s.control(id, "paused", func(e *entry) (func(*Task), bool) {
		ok := (e.snap.State == StateRunning || e.snap.State == StateWaiting) && !e.retryPending()
		return e.spec.OnPause, ok
	})
}

// Resume asks to resume a paused task through its OnResume.
func (s *Service) Resume(id string) error {
	return s.control(id, "resumed", func(e *entry) (func(*Task), bool) {
		return e.spec.OnResume, e.snap.State == StatePaused
	})
}

func (s *Service) control(id, verb string, pick func(*entry) (func(*Task), bool)) error {
	s.mu.Lock()
	e, err := s.activeLocked(id)
	var fn func(*Task)
	var h *Task
	if err == nil {
		var valid bool
		fn, valid = pick(e)
		switch {
		case fn == nil:
			err = errors.New(ErrUnsupported, fmt.Sprintf("task %q can't be %s", id, verb), nil)
		case !valid:
			err = errors.New(ErrInvalidState, fmt.Sprintf("task %q is %s and can't be %s", id, e.snap.State, verb), nil)
		default:
			h = e.current
		}
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	fn(h)
	return nil
}

// Retry runs a task again now: a Failed task, or one waiting for an
// automatic retry ("Retry now"). The backoff carries on from where it was if
// this attempt fails too. OnRetry gets the new attempt's handle.
func (s *Service) Retry(id string) error {
	s.mu.Lock()
	e, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return errors.New(ErrNotFound, fmt.Sprintf("task %q is not in the list", id), nil)
	}
	if e.spec.OnRetry == nil {
		s.mu.Unlock()
		return errors.New(ErrUnsupported, fmt.Sprintf("task %q can't be retried", id), nil)
	}
	if e.snap.State != StateFailed && !e.retryPending() {
		s.mu.Unlock()
		return errors.New(ErrInvalidState, fmt.Sprintf("task %q is %s, not failed", id, e.snap.State), nil)
	}
	h := s.beginAttemptLocked(e)
	s.changedLocked(true)
	s.mu.Unlock()
	e.spec.OnRetry(h)
	return nil
}

// Dismiss removes a finished task, and its parts, from the list.
func (s *Service) Dismiss(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.tasks[id]
	if !ok {
		return errors.New(ErrNotFound, fmt.Sprintf("task %q is not in the list", id), nil)
	}
	family := s.familyLocked(e)
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
	var remove []*entry
	for _, e := range s.order {
		if e.snap.Parent != "" {
			continue
		}
		family := s.familyLocked(e)
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
	for _, e := range s.order {
		s.stopClocksLocked(e)
		stopTimer(&e.retryTimer)
	}
}

// Wake starts every clock again from zero: each running task's timeout on
// silence and switch to Waiting restart, and a task waiting for an automatic
// retry drops its backoff and tries at once (its attempt count, for
// MaxAttempts, stays). Call it on wake (Wails reports it on Windows);
// elsewhere the service detects wake from a jump in the clock.
func (s *Service) Wake() {
	s.mu.Lock()
	retries := s.wakeLocked()
	s.mu.Unlock()
	for _, h := range retries {
		h.e.spec.OnRetry(h)
	}
}

func (s *Service) wakeLocked() []*Task {
	s.asleep = false
	var retries []*Task
	for _, e := range s.order {
		if e.snap.State.Finished() {
			continue
		}
		if e.retryPending() {
			e.backoffStep = 0
			retries = append(retries, s.beginAttemptLocked(e))
			continue
		}
		s.armClocksLocked(e)
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
	for _, e := range s.order {
		s.stopClocksLocked(e)
		stopTimer(&e.retryTimer)
	}
	stopTimer(&s.detector)
	if s.flush != nil {
		s.changedLocked(true)
	}
	s.mu.Unlock()
	s.baseCancel()
}

// --- internals (all called with s.mu held) ---

func (e *entry) retryPending() bool {
	return e.snap.State == StateWaiting && e.snap.NextTry != nil
}

// newHandleLocked starts a new attempt's handle, with its own context, and
// makes every earlier handle stale.
func (s *Service) newHandleLocked(e *entry) *Task {
	if e.current != nil {
		e.current.cancel()
	}
	e.attempt++
	ctx, cancel := context.WithCancel(s.baseCtx)
	h := &Task{svc: s, e: e, attempt: e.attempt, ctx: ctx, cancel: cancel}
	e.current = h
	return h
}

// liveLocked reports whether a report through h counts: the task is in the
// list and active, h is its current attempt, and it isn't waiting for an
// automatic retry (the attempt that failed is over).
func (s *Service) liveLocked(h *Task) bool {
	e := h.e
	return !s.closed && !e.removed && !e.snap.State.Finished() &&
		h.attempt == e.attempt && !e.retryPending()
}

func (s *Service) activeLocked(id string) (*entry, error) {
	e, ok := s.tasks[id]
	if !ok {
		return nil, errors.New(ErrNotFound, fmt.Sprintf("task %q is not in the list", id), nil)
	}
	if e.snap.State.Finished() {
		return nil, errors.New(ErrInvalidState, fmt.Sprintf("task %q has finished", id), nil)
	}
	return e, nil
}

func (s *Service) snapshotsLocked() []Snapshot {
	out := make([]Snapshot, len(s.order))
	for i, e := range s.order {
		out[i] = e.snap
	}
	return out
}

// familyLocked returns e and every part under it.
func (s *Service) familyLocked(e *entry) []*entry {
	family := []*entry{e}
	for i := 0; i < len(family); i++ {
		for _, c := range s.order {
			if c.snap.Parent == family[i].snap.ID {
				family = append(family, c)
			}
		}
	}
	return family
}

func (s *Service) removeLocked(remove []*entry) {
	gone := make(map[*entry]bool, len(remove))
	for _, e := range remove {
		gone[e] = true
		delete(s.tasks, e.snap.ID)
		s.stopClocksLocked(e)
		stopTimer(&e.retryTimer)
		e.removed = true
		e.current.cancel()
	}
	kept := s.order[:0]
	for _, e := range s.order {
		if !gone[e] {
			kept = append(kept, e)
		}
	}
	for i := len(kept); i < len(s.order); i++ {
		s.order[i] = nil
	}
	s.order = kept
}

// setRunningLocked moves the task to Running. Leaving Waiting restores the
// task's own timeout, which SetTimeout(NoTimeout) may have turned off.
func (s *Service) setRunningLocked(e *entry) {
	if e.snap.State == StateWaiting {
		e.timeout = e.spec.Timeout
	}
	e.snap.State = StateRunning
	e.autoWaiting = false
	if e.snap.StartedAt == nil {
		e.snap.StartedAt = millisPtr(s.clock.Now())
	}
}

// touchLocked records a report and restarts the task's clocks.
func (s *Service) touchLocked(e *entry) {
	e.snap.UpdatedAt = millis(s.clock.Now())
	s.armClocksLocked(e)
}

func (s *Service) stopClocksLocked(e *entry) {
	stopTimer(&e.timer)
	stopTimer(&e.quiet)
	e.gen++
}

// armClocksLocked (re)starts the task's timeout on silence, which runs while
// it is Running, or Waiting on its own work (not on a retry), and its switch
// to Waiting, which runs while it is Running.
func (s *Service) armClocksLocked(e *entry) {
	s.stopClocksLocked(e)
	if s.closed || s.asleep || e.removed {
		return
	}
	gen := e.gen
	if e.snap.State == StateRunning && s.waitingAfter > 0 {
		e.quiet = s.clock.AfterFunc(s.waitingAfter, func() { s.onQuiet(e, gen) })
	}
	if e.snap.State != StateRunning && (e.snap.State != StateWaiting || e.retryPending()) {
		return
	}
	d := e.timeout
	if d == 0 {
		d = s.timeout
	}
	if d <= 0 {
		return
	}
	e.timer = s.clock.AfterFunc(d, func() { s.onTimeout(e, gen, d) })
}

// onQuiet moves a running task that hasn't reported for a while to Waiting.
// Its timeout on silence keeps counting from the last report.
func (s *Service) onQuiet(e *entry, gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.gen != gen || e.removed || s.closed || e.snap.State != StateRunning {
		return
	}
	e.quiet = nil
	e.snap.State = StateWaiting
	e.autoWaiting = true
	if e.spec.WaitingWords != "" {
		e.snap.Words = e.spec.WaitingWords
	}
	e.snap.UpdatedAt = millis(s.clock.Now())
	s.changedLocked(true)
}

func (s *Service) onTimeout(e *entry, gen uint64, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.gen != gen || e.removed || s.closed || e.snap.State.Finished() || e.retryPending() {
		return
	}
	e.timer = nil
	err := errors.New(errors.ErrTimeout, fmt.Sprintf("task %q made no progress for %s", e.snap.ID, d), nil)
	s.failLocked(e, err, s.timeoutWords(d))
}

// failLocked fails the current attempt. An automatic retry policy waits
// with a backoff instead, if the task can be retried.
func (s *Service) failLocked(e *entry, err error, words string) {
	info := errorInfo(err)
	e.attempts++
	if e.spec.Retry == RetryAuto && e.spec.OnRetry != nil &&
		(e.spec.MaxAttempts == 0 || e.attempts < e.spec.MaxAttempts) {
		e.backoffStep++
		wait := s.backoffFor(e.backoffStep)
		now := s.clock.Now()
		s.stopClocksLocked(e)
		e.current.cancel()
		e.snap.State = StateWaiting
		e.autoWaiting = false
		e.snap.Error = info
		e.snap.Words = words
		e.snap.NextTry = millisPtr(now.Add(wait))
		e.snap.UpdatedAt = millis(now)
		stopTimer(&e.retryTimer)
		e.retryGen++
		gen := e.retryGen
		if !s.asleep && !s.closed {
			e.retryTimer = s.clock.AfterFunc(wait, func() { s.autoRetry(e, gen) })
		}
		s.changedLocked(true)
		return
	}
	s.finishLocked(e, StateFailed, words, info)
	s.changedLocked(true)
}

func (s *Service) autoRetry(e *entry, gen uint64) {
	s.mu.Lock()
	if e.retryGen != gen || e.removed || s.closed || !e.retryPending() {
		s.mu.Unlock()
		return
	}
	e.retryTimer = nil
	h := s.beginAttemptLocked(e)
	s.changedLocked(true)
	s.mu.Unlock()
	e.spec.OnRetry(h)
}

// beginAttemptLocked starts a fresh attempt and returns its handle: Running,
// a new context, the error and next try cleared, and the words back to the
// task's own.
func (s *Service) beginAttemptLocked(e *entry) *Task {
	stopTimer(&e.retryTimer)
	e.retryGen++
	h := s.newHandleLocked(e)
	s.setRunningLocked(e)
	e.snap.Error = nil
	e.snap.NextTry = nil
	e.snap.Words = e.spec.Words
	s.touchLocked(e)
	s.armDetectorLocked()
	return h
}

func (s *Service) finishLocked(e *entry, state State, words string, info *ErrorInfo) {
	s.stopClocksLocked(e)
	stopTimer(&e.retryTimer)
	e.retryGen++
	e.current.cancel()
	e.autoWaiting = false
	e.snap.State = state
	e.snap.Words = words
	e.snap.Error = info
	e.snap.NextTry = nil
	e.snap.UpdatedAt = millis(s.clock.Now())
}

func (s *Service) backoffFor(step int) time.Duration {
	i := step - 1
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
	for _, e := range s.order {
		if !e.snap.State.Finished() {
			s.armDetectorLocked()
			break
		}
	}
	s.mu.Unlock()
	for _, h := range retries {
		h.e.spec.OnRetry(h)
	}
}

func stopTimer(t *Timer) {
	if *t != nil {
		(*t).Stop()
		*t = nil
	}
}
