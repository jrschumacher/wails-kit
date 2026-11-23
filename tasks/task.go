package tasks

import (
	"context"
	"time"
)

// Task is the handle the app's work reports through. Its methods mirror
// wkTask's. Reports after the task has finished, been dismissed, or the
// service has closed are ignored, so work that ends after a timeout or a
// Stop can't bring a task back. Its methods are safe for concurrent use.
type Task struct {
	svc  *Service
	spec Spec

	// Guarded by svc.mu.
	snap       Snapshot
	timeout    time.Duration
	timer      Timer // timeout on silence
	retryTimer Timer
	gen        uint64
	attempts   int
	removed    bool
	ctx        context.Context
	cancel     context.CancelFunc
}

// ID returns the task's ID.
func (t *Task) ID() string { return t.spec.ID }

// Context returns the context of the task's current attempt. It is cancelled
// when the attempt ends: the task finishes, times out, waits for a retry, is
// dismissed, or is asked to stop without an OnStop. A retry gets a new one.
func (t *Task) Context() context.Context {
	t.svc.mu.Lock()
	defer t.svc.mu.Unlock()
	return t.ctx
}

// Snapshot returns the task as tasks:changed sends it.
func (t *Task) Snapshot() Snapshot {
	t.svc.mu.Lock()
	defer t.svc.mu.Unlock()
	return t.snap
}

// AddPart adds a task that is part of this one. Parts show their own state;
// the app reports the whole task Done only once every part is.
func (t *Task) AddPart(spec Spec) (*Task, error) {
	spec.Parent = t.spec.ID
	return t.svc.Add(spec)
}

// update runs fn on a live task with the service locked, then sends a
// snapshot: at once if fn changed the state, coalesced otherwise.
func (t *Task) update(fn func() bool) {
	s := t.svc
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || t.removed || t.snap.State.Finished() {
		return
	}
	before := t.snap.State
	if !fn() {
		return
	}
	s.touchLocked(t)
	s.armDetectorLocked()
	s.changedLocked(t.snap.State != before)
}

// setState moves to state, with words when they're given.
func (t *Task) setState(state State, words string) {
	t.update(func() bool {
		if t.snap.NextTry != nil {
			// Waiting for an automatic retry: only Retry, Stop or the
			// timer move it on.
			return false
		}
		t.snap.State = state
		if words != "" {
			t.snap.Words = words
		}
		if state == StateRunning && t.snap.StartedAt == nil {
			t.snap.StartedAt = millisPtr(t.svc.clock.Now())
		}
		return true
	})
}

// Running moves the task to Running: from Queued once it starts, or back
// from Waiting, Paused or Stalled. Empty words keep the current ones.
func (t *Task) Running(words string) { t.setState(StateRunning, words) }

// Queued moves the task to Queued: it can't start yet ("Queued · after 2
// others"). No timeout runs while queued.
func (t *Task) Queued(words string) { t.setState(StateQueued, words) }

// Waiting says what the work is waiting for ("Waiting for GitHub…"). It is
// neutral, and the timeout keeps running unless the task turns it off with
// SetTimeout(NoTimeout).
func (t *Task) Waiting(words string) { t.setState(StateWaiting, words) }

// Paused moves the task to Paused, with what's done so far. No timeout runs
// while paused.
func (t *Task) Paused(words string) { t.setState(StatePaused, words) }

// Stalled says the task can't go on without a person, and what to do ("GitHub
// sign-in expired · Sign in"). No timeout runs while stalled.
func (t *Task) Stalled(words string) { t.setState(StateStalled, words) }

// Progress reports progress. It restarts the timeout on silence, and moves a
// Waiting task back to Running. Snapshots for progress alone are coalesced.
func (t *Task) Progress(p Progress) {
	t.update(func() bool {
		if t.snap.NextTry != nil {
			return false
		}
		if t.snap.State == StateWaiting {
			t.snap.State = StateRunning
		}
		t.snap.Done = p.Done
		t.snap.Total = p.Total
		t.snap.Unit = p.Unit
		t.snap.Step = p.Step
		t.snap.Steps = p.Steps
		t.snap.Stage = p.Stage
		return true
	})
}

// SetWords changes the words without changing the state. Like progress, it
// counts as a sign of life and is coalesced.
func (t *Task) SetWords(words string) {
	t.update(func() bool {
		t.snap.Words = words
		return true
	})
}

// SetTimeout changes the task's timeout on silence: 0 uses the service's,
// NoTimeout turns it off (while it waits on something it has named).
func (t *Task) SetTimeout(d time.Duration) {
	s := t.svc
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || t.removed || t.snap.State.Finished() {
		return
	}
	t.timeout = d
	if t.snap.NextTry == nil {
		s.armSilenceLocked(t)
	}
}

func (t *Task) finish(state State, words string) {
	s := t.svc
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || t.removed || t.snap.State.Finished() {
		return
	}
	s.finishLocked(t, state, words, nil)
	s.changedLocked(true)
}

// Done finishes the task. Words are the outcome ("Repo added"); they're
// always announced, and shown where the result isn't visible.
func (t *Task) Done(words string) { t.finish(StateDone, words) }

// Cancelled finishes the task as cancelled: the app has rolled back to the
// state from before.
func (t *Task) Cancelled() { t.finish(StateCancelled, "") }

// Stopped finishes the task as stopped, with what finished ("12 of 40 files
// copied").
func (t *Task) Stopped(words string) { t.finish(StateStopped, words) }

// Fail fails the current attempt with err and the app's words ("Couldn't
// save the file"). The snapshot's error carries err's code and its message
// from errors.RegisterMessages.
//
// With RetryAuto (and an OnRetry), the task waits instead: Waiting, with the
// error and nextTry set, until the backoff runs out and OnRetry runs.
// Otherwise it is Failed until the person retries or dismisses it.
func (t *Task) Fail(err error, words string) {
	s := t.svc
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || t.removed || t.snap.State.Finished() || t.snap.NextTry != nil {
		return
	}
	s.failLocked(t, err, words)
}

// retryPendingLocked reports whether the task is waiting for an automatic
// retry.
func (t *Task) retryPendingLocked() bool {
	return t.snap.State == StateWaiting && t.snap.NextTry != nil
}
