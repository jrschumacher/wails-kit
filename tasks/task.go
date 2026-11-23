package tasks

import (
	"context"
	"time"
)

// Task is the handle one attempt of a task reports through. Its methods
// mirror wkTask's. Add returns the first attempt's handle, and each retry
// passes a new one to OnRetry. Reports are ignored when they come through
// an earlier attempt's handle, while the task waits for an automatic retry,
// or after it has finished, been dismissed or the service has closed, so
// work that ends late can't change the task. Its methods are safe for
// concurrent use.
type Task struct {
	svc     *Service
	e       *entry
	attempt uint64
	ctx     context.Context
	cancel  context.CancelFunc
}

// ID returns the task's ID. Every attempt's handle has the same one.
func (t *Task) ID() string { return t.e.spec.ID }

// Context returns this attempt's context. It is cancelled when the attempt
// ends: the task finishes, times out, waits for a retry, is retried, is
// dismissed, or is asked to stop without an OnStop.
func (t *Task) Context() context.Context { return t.ctx }

// Snapshot returns the task as tasks:changed sends it.
func (t *Task) Snapshot() Snapshot {
	t.svc.mu.Lock()
	defer t.svc.mu.Unlock()
	return t.e.snap
}

// AddPart adds a task that is part of this one. Parts show their own state;
// the app reports the whole task Done only once every part is.
func (t *Task) AddPart(spec Spec) (*Task, error) {
	spec.Parent = t.e.spec.ID
	return t.svc.Add(spec)
}

// update runs fn on the task if a report through t counts, then restarts its
// clocks and sends a snapshot: at once if the state changed, coalesced
// otherwise.
func (t *Task) update(fn func(e *entry)) {
	s := t.svc
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.liveLocked(t) {
		return
	}
	e := t.e
	before := e.snap.State
	fn(e)
	s.touchLocked(e)
	s.armDetectorLocked()
	s.changedLocked(e.snap.State != before)
}

// setState moves to state, with words when they're given.
func (t *Task) setState(state State, words string) {
	t.update(func(e *entry) {
		if state == StateRunning {
			t.svc.setRunningLocked(e)
		} else {
			e.snap.State = state
			e.autoWaiting = false
		}
		if words != "" {
			e.snap.Words = words
		}
	})
}

// Running moves the task to Running: from Queued once it starts, or back
// from Waiting, Paused or Stalled. Empty words keep the current ones.
func (t *Task) Running(words string) { t.setState(StateRunning, words) }

// Queued moves the task to Queued: it can't start yet ("Queued · after 2
// others"). No clocks run while queued.
func (t *Task) Queued(words string) { t.setState(StateQueued, words) }

// Waiting says what the work is waiting for ("Waiting for GitHub…"). It is
// neutral, and the timeout keeps running unless the task turns it off with
// SetTimeout(NoTimeout).
func (t *Task) Waiting(words string) { t.setState(StateWaiting, words) }

// Paused moves the task to Paused, with what's done so far. No clocks run
// while paused.
func (t *Task) Paused(words string) { t.setState(StatePaused, words) }

// Stalled says the task can't go on without a person, and what to do ("GitHub
// sign-in expired · Sign in"). No clocks run while stalled.
func (t *Task) Stalled(words string) { t.setState(StateStalled, words) }

// Progress reports progress. It restarts the clocks, and moves a Waiting
// task back to Running. Snapshots for progress alone are coalesced.
func (t *Task) Progress(p Progress) {
	t.update(func(e *entry) {
		if e.snap.State == StateWaiting {
			t.svc.setRunningLocked(e)
		}
		e.snap.Done = p.Done
		e.snap.Total = p.Total
		e.snap.Unit = p.Unit
		e.snap.Step = p.Step
		e.snap.Steps = p.Steps
		e.snap.Stage = p.Stage
	})
}

// SetWords changes the words. Like progress, it is a sign of life: it
// restarts the clocks and moves a task the service put in Waiting back to
// Running (a task the app put in Waiting stays there). Coalesced.
func (t *Task) SetWords(words string) {
	t.update(func(e *entry) {
		if e.autoWaiting {
			t.svc.setRunningLocked(e)
		}
		e.snap.Words = words
	})
}

// SetTimeout changes the task's timeout on silence: 0 uses the service's,
// NoTimeout turns it off while it waits on something it has named. When the
// task next goes from Waiting to Running, its own timeout (Spec.Timeout)
// comes back.
func (t *Task) SetTimeout(d time.Duration) {
	s := t.svc
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.liveLocked(t) {
		return
	}
	t.e.timeout = d
	s.armClocksLocked(t.e)
}

func (t *Task) finish(state State, words string) {
	s := t.svc
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.liveLocked(t) {
		return
	}
	s.finishLocked(t.e, state, words, nil)
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

// Fail fails this attempt with err and the app's words ("Couldn't save the
// file"). The snapshot's error carries err's code and its message from
// errors.RegisterMessages.
//
// With RetryAuto (and an OnRetry), the task waits instead: Waiting, with the
// error and nextTry set, until the backoff runs out and OnRetry runs with a
// new handle. Otherwise it is Failed until the person retries or dismisses
// it.
func (t *Task) Fail(err error, words string) {
	s := t.svc
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.liveLocked(t) {
		return
	}
	s.failLocked(t.e, err, words)
}
