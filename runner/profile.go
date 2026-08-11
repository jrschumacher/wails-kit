package runner

import (
	"math/rand"
	"time"
)

// Profile bundles the handful of policy knobs a queue needs — persistence,
// retry, backpressure, retention, and sleep/wake behavior — behind one of
// three named starting points. Every field is exported and overridable:
//
//	p := runner.Durable()
//	p.MaxAttempts = 10
//
// Prefer overriding one or two fields on a profile over hand-assembling a
// Profile from scratch; the zero value of most fields (e.g. Backoff == nil)
// is not a sensible default on its own — New fills it in from
// DefaultBackoff, but only when it is nil.
type Profile struct {
	// Name identifies the profile for logging/diagnostics. Not load-bearing
	// for behavior.
	Name string
	// Persistence selects in-memory (PersistNone) vs a Store (PersistStore).
	Persistence Persistence
	// MaxAttempts caps how many times a job may be dispatched. 0 means
	// unlimited (a Durable job retries forever, subject to DeadLetter once
	// it would otherwise be dropped — see the state-transition table on
	// JobState). N > 0 means "drop (or dead-letter) after N attempts".
	MaxAttempts int
	// Backoff computes the delay before the next attempt, given the
	// 1-indexed attempt number that just failed. New defaults this to
	// DefaultBackoff when nil.
	Backoff func(attempt int) time.Duration
	// DeadLetter, when true, retains a terminally-failed job in
	// JobStateDead (queryable via Queue.Dead, re-enqueueable via
	// Queue.Requeue) instead of dropping it to JobStateFailed.
	DeadLetter bool
	// ResumeOnWake, when true, emits EventWake when Queue.Start detects a
	// wall-clock gap larger than 2x its tick interval between ticks (see
	// EventWake). The due-scan itself always happens every tick regardless
	// of this field — ResumeOnWake only gates the informational event, not
	// whether due jobs get processed.
	ResumeOnWake bool
	// MaxQueueDepth caps pending+running jobs; Enqueue returns
	// ErrQueueFull beyond it. 0 means unbounded.
	MaxQueueDepth int
	// Retention is how long a completed job's record is kept in the Store
	// after it reaches a terminal, non-dead-lettered state, before Sweep
	// removes it. 0 means "delete on success" (eligible for removal on the
	// very next Sweep pass). Dead-lettered jobs (JobStateDead) are never
	// swept regardless of Retention — DeadLetter controls their lifetime.
	Retention time.Duration
}

// Ephemeral returns an in-memory profile: nothing survives process exit,
// jobs are dropped (not dead-lettered) after 3 failed attempts. Suited to
// work that is fine to lose — a UI toast retry, a debounced re-render.
func Ephemeral() Profile {
	return Profile{
		Name:         "ephemeral",
		Persistence:  PersistNone,
		MaxAttempts:  3,
		DeadLetter:   false,
		ResumeOnWake: true,
	}
}

// Durable returns a persisted, at-least-once profile: unlimited retries
// with backoff, dead-lettered (not dropped) once an app-supplied
// MaxAttempts override would otherwise apply, idempotency keys for
// dedupe. Suited to expensive, side-effecting work — LLM calls, syncs,
// anything a user would be upset to silently lose.
func Durable() Profile {
	return Profile{
		Name:         "durable",
		Persistence:  PersistStore,
		MaxAttempts:  0,
		DeadLetter:   true,
		ResumeOnWake: true,
	}
}

// BestEffort returns a persisted profile that survives a restart but gives
// up (drops, does not dead-letter) after MaxAttempts failures. Suited to
// work that's worth surviving a crash but not worth an indefinite retry
// queue — a background thumbnail regeneration, a non-critical sync.
func BestEffort() Profile {
	return Profile{
		Name:         "best_effort",
		Persistence:  PersistStore,
		MaxAttempts:  5,
		DeadLetter:   false,
		ResumeOnWake: true,
	}
}

// DefaultBackoff is exponential from 1s, capped at 5m, with up to 50%
// jitter: base = min(1s * 2^(attempt-1), 5m); delay is uniformly random in
// [base/2, base]. attempt is 1-indexed (the attempt number that just
// failed). attempt <= 0 is treated as 1.
func DefaultBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	const (
		base = time.Second
		max  = 5 * time.Minute
	)
	// Cap the shift so 1<<n never overflows before the max clamp applies.
	shift := attempt - 1
	if shift > 20 {
		shift = 20
	}
	d := base * time.Duration(int64(1)<<uint(shift))
	if d > max || d <= 0 {
		d = max
	}
	half := d / 2
	//nolint:gosec // jitter timing, not a security boundary
	return half + time.Duration(rand.Int63n(int64(half+1)))
}
