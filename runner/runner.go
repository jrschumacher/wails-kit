// Package runner is a durable job queue plus a worker that ticks —
// Watermill-inspired (message/handler separation, middleware-style retry),
// not Temporal-style durable execution. It is deliberately last-priority
// kit infrastructure: a reskinned web-app shell may never need it, while it
// needs appearance/windowstate/health immediately.
//
// A developer picks a Profile (Ephemeral, Durable, or BestEffort), overrides
// the one or two fields they care about, and never thinks about the rest:
//
//	p := runner.Durable()
//	p.MaxAttempts = 10
//	q, err := runner.New(p, runner.WithStore(store))
//	q.Handle("send_email", handleSendEmail)
//	id, err := q.Enqueue(ctx, "send_email", payload)
//	go q.Start(ctx)
//
// At-least-once delivery. LLM calls and network work are expensive and
// side-effecting; at-most-once loses work and exactly-once is a lie. A
// crash between a handler returning success and the queue recording it can
// cause a job to run again — that is the deliberate trade the "at least"
// makes. Handlers that cannot tolerate a duplicate run must declare an
// EnqueueOption WithIdempotencyKey and make the *handler's own side effect*
// idempotent against it; the queue's own idempotency-key dedupe only
// prevents duplicate *enqueues* of the same logical job while it is
// pending/running, not duplicate executions after crash recovery.
package runner

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/i18n"
)

// Persistence selects where a Queue's jobs live.
type Persistence int

const (
	// PersistNone keeps jobs in memory only — dropped on process exit.
	PersistNone Persistence = iota
	// PersistStore persists jobs via a Store (see WithStore). Required for
	// Durable and BestEffort profiles.
	PersistStore
)

// JobState is a job's position in its lifecycle.
//
// Transitions:
//
//	pending -> running            (dispatch, see Queue.dispatch)
//	running -> done                (handler succeeded)
//	running -> pending             (handler failed, attempts remain — retry)
//	running -> failed              (handler failed, attempts exhausted, DeadLetter=false — dropped)
//	running -> dead                (handler failed, attempts exhausted, DeadLetter=true — retained)
//	dead    -> pending             (Queue.Requeue)
type JobState string

const (
	JobStatePending JobState = "pending"
	JobStateRunning JobState = "running"
	JobStateDone    JobState = "done"
	JobStateFailed  JobState = "failed"
	JobStateDead    JobState = "dead"
)

// Job is one unit of work. The JSON tags are load-bearing: runner/flatfile
// stores a Job per line verbatim (field names, not renamed/abbreviated) so
// that `git diff` on a queue file means something to a human or an LLM
// reading it. Payload is always raw JSON — never base64.
type Job struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	State          JobState        `json:"state"`
	Attempts       int             `json:"attempts"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	EnqueuedAt     time.Time       `json:"enqueued_at"`
	// NextRunAt is when a pending job becomes due. For a job that has
	// reached a terminal state (done/failed/dead), Store implementations
	// and Queue repurpose this same field to record the transition
	// timestamp — the authoritative Job shape has no separate
	// CompletedAt/UpdatedAt field, and Retention/Sweep need *some*
	// timestamp to measure age against. See runner/AGENTS.md "Landmines".
	NextRunAt time.Time `json:"next_run_at"`
	LastError string    `json:"last_error,omitempty"`
}

// Handler processes one job. It must respect ctx cancellation — Queue.Start
// cancels every in-flight handler's context when its own ctx is done, and
// waits for all handlers to return before Start returns. A Handler that
// ignores ctx and blocks forever will block that shutdown and leak its
// goroutine; this is a Handler contract violation, not a Queue bug (the
// same contract health.Probe documents for the same reason).
type Handler func(ctx context.Context, job Job) error

// Store persists jobs. Implementations: runner/flatfile (JSONL, human
// readable — the kit default) and runner/sqlitestore (WP-41).
//
// Due's contract: it returns up to limit jobs that are either (a) State ==
// JobStatePending with NextRunAt <= now, or (b) State == JobStateRunning
// regardless of NextRunAt. Case (b) is what makes crash recovery possible
// without a dedicated "list running" method — a job a Store reports as
// still "running" is, from that Store's point of view, either genuinely
// in-flight in this same process (Queue tracks that separately in memory
// and skips it) or orphaned by an unclean shutdown of a *previous* process
// (Queue redispatches it, incrementing Attempts). Ordering must be
// deterministic (earliest NextRunAt, then EnqueuedAt, then ID) so tests and
// `git diff` on a flat-file store stay stable.
type Store interface {
	// Append adds a newly enqueued job. The job's ID is unique; Append on a
	// duplicate ID is a programmer error and may return an error or panic
	// depending on the implementation.
	Append(job Job) error
	// Update persists a job's current state in place (by ID). Update on an
	// unknown ID returns an error.
	Update(job Job) error
	// Due returns jobs ready to be dispatched — see the Due contract above.
	// limit <= 0 means "no limit".
	Due(now time.Time, limit int) ([]Job, error)
	// Sweep removes terminal jobs (done, and failed when not dead-lettered)
	// whose NextRunAt (repurposed as completion time — see Job.NextRunAt)
	// is older than now.Add(-retention). retention <= 0 means "eligible
	// immediately" (delete on success), not "keep forever" — a Store that
	// wants to keep dead-lettered jobs forever does so by never sweeping
	// JobStateDead, not by special-casing retention.
	Sweep(retention time.Duration, now time.Time) error
	// Close releases any resources held by the Store (file handles, DB
	// connections). Queue.Close calls this once, after draining in-flight
	// handlers.
	Close() error
}

// Lister is an optional Store capability: list every job currently in the
// given state. Both runner's built-in memory store and runner/flatfile
// implement it. Queue uses it, when available, to give MaxQueueDepth
// accounting, idempotency-key dedupe, and Dead() real cross-restart
// fidelity; a minimal custom Store that only implements the required Store
// methods still works, but MaxQueueDepth/idempotency bookkeeping and Dead()
// then only reflect what has happened since the current process started —
// see runner/AGENTS.md "Landmines".
type Lister interface {
	List(state JobState) ([]Job, error)
}

// Deleter is an optional Store capability: permanently remove a job's
// record by ID. Both runner's built-in memory store and runner/flatfile
// (and runner/sqlitestore) implement it. Queue.Discard uses it to actually
// purge a dead-lettered job's Store record — without a Deleter, a Store
// has no way to remove a record at all (Sweep only ever removes
// done/failed jobs past Retention; it deliberately never touches
// JobStateDead, since DeadLetter/Retention govern different lifetimes —
// see Sweep's doc comment), so a dead job on such a Store would otherwise
// be stuck forever. Discard returns ErrDiscardUnsupported when the Store
// doesn't implement Deleter, rather than silently succeeding while leaving
// the record behind.
type Deleter interface {
	Delete(id string) error
}

// Event names. All three fire only on the actual state transition named
// (never on every attempt, never on every tick) — a job that fails but has
// attempts remaining transitions running -> pending and emits nothing.
const (
	EventJobDone   = "runner:job_done"
	EventJobFailed = "runner:job_failed"
	EventJobDead   = "runner:job_dead"
	// EventWake fires when Queue.Start detects a wall-clock jump larger
	// than 2x its tick interval between two ticks (a sleep/wake gap) and
	// the active Profile has ResumeOnWake set. It is purely informational;
	// the due-scan that follows a gap is the same single scan every tick
	// performs, not an extra "catch-up" pass — see runner/AGENTS.md.
	EventWake = "runner:wake"
)

// JobEventPayload is the payload for EventJobDone, EventJobFailed, and
// EventJobDead.
type JobEventPayload struct {
	Job Job `json:"job"`
}

// WakePayload is the payload for EventWake.
type WakePayload struct {
	Gap time.Duration `json:"gap"`
}

// Error codes.
const (
	ErrQueueFull          errors.Code = "runner_queue_full"
	ErrNoHandler          errors.Code = "runner_no_handler"
	ErrStoreRequired      errors.Code = "runner_store_required"
	ErrJobNotFound        errors.Code = "runner_job_not_found"
	ErrQueueClosed        errors.Code = "runner_queue_closed"
	ErrDiscardUnsupported errors.Code = "runner_discard_unsupported"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrQueueFull:     i18n.T("wailskit.runner.errors.queue_full", "The background job queue is full. Please try again shortly."),
		ErrNoHandler:     i18n.T("wailskit.runner.errors.no_handler", "This job type isn't handled yet."),
		ErrStoreRequired: i18n.T("wailskit.runner.errors.store_required", "The background job queue is misconfigured."),
		ErrJobNotFound:   i18n.T("wailskit.runner.errors.job_not_found", "The requested job was not found."),
		ErrQueueClosed:   i18n.T("wailskit.runner.errors.queue_closed", "The background job queue has been shut down."),
		ErrDiscardUnsupported: i18n.T("wailskit.runner.errors.discard_unsupported",
			"This job queue's storage doesn't support permanently discarding a job."),
	})
}
