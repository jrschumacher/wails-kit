// Package tasks keeps the list of an app's tasks on the Go side, so it
// survives a reloaded web view and closed windows, and emits one event,
// tasks:changed, with a snapshot of the whole list.
//
// It implements "Go task events" in frontend/ui/BEHAVIOUR.md: the lifecycle
// states, progress coalesced to at most four snapshots a second, a timeout on
// silence per running task, retry policies with backoff and jitter, and clocks
// that start again on wake.
package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jrschumacher/wails-kit/errors"
)

// EventChanged is the one event the service emits. Its payload is
// ChangedPayload: a snapshot of every task in the list.
const EventChanged = "tasks:changed"

// State is a task's lifecycle state.
type State string

// The lifecycle states. Done, Failed, Cancelled and Stopped are finished: the
// task stays in the list until it is dismissed (or retried, for Failed).
const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateWaiting   State = "waiting"
	StatePaused    State = "paused"
	StateStalled   State = "stalled"
	StateDone      State = "done"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
	StateStopped   State = "stopped"
)

// States lists every state, in lifecycle order.
var States = []State{
	StateQueued, StateRunning, StateWaiting, StatePaused, StateStalled,
	StateDone, StateFailed, StateCancelled, StateStopped,
}

// Finished reports whether the state is an end state.
func (s State) Finished() bool {
	switch s {
	case StateDone, StateFailed, StateCancelled, StateStopped:
		return true
	}
	return false
}

// RetryPolicy says whether a failed task is safe to retry by itself.
type RetryPolicy string

const (
	// RetryAuto retries by itself after a backoff (a read, a sync, an upload
	// with an idempotency key). While it waits, the task is Waiting with
	// nextTry set: neutral, since it may still finish by itself.
	RetryAuto RetryPolicy = "auto"
	// RetryManual fails and waits for the person to retry. The default.
	RetryManual RetryPolicy = "manual"
	// RetryUnsafe is a write that may or may not have happened. It is never
	// retried by itself; the app checks, then offers Retry.
	RetryUnsafe RetryPolicy = "unsafe"
)

// StopMode says how a task can be stopped. It matches wkTask's stop option.
// The zero value means it can't be stopped, and is sent as false.
type StopMode string

const (
	StopNone     StopMode = ""
	StopCancel   StopMode = "cancel"    // returns to the state from before
	StopStop     StopMode = "stop"      // keeps what finished
	StopStopAsks StopMode = "stop-asks" // Stop, after an Alert
	StopPause    StopMode = "pause"     // can pause and resume
	StopUnsafe   StopMode = "unsafe"    // stopping may leave work half done
)

// MarshalJSON sends StopNone as false and every other mode as its string.
func (m StopMode) MarshalJSON() ([]byte, error) {
	if m == StopNone {
		return []byte("false"), nil
	}
	return json.Marshal(string(m))
}

// UnmarshalJSON accepts false (or null) as StopNone, or a mode string.
func (m *StopMode) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "false", "null":
		*m = StopNone
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("tasks: stop mode: %w", err)
	}
	*m = StopMode(s)
	return nil
}

func (m StopMode) valid() bool {
	switch m {
	case StopNone, StopCancel, StopStop, StopStopAsks, StopPause, StopUnsafe:
		return true
	}
	return false
}

// ErrorInfo is a failed task's error. UserMsg comes from
// errors.RegisterMessages; Message is the raw error, for an ErrorBox.
type ErrorInfo struct {
	Code    errors.Code `json:"code"`
	UserMsg string      `json:"userMsg"`
	Message string      `json:"message"`
}

// Snapshot is one task as tasks:changed sends it. Its shape is a contract
// shared with @wails-kit/types (TaskSnapshot): fields are only ever added.
//
// Times are Unix milliseconds. A null (nil) field is unknown or not set.
type Snapshot struct {
	ID      string `json:"id"`
	Parent  string `json:"parent"`  // the task this is a part of; "" for a top-level task
	Subject string `json:"subject"` // what it acts on, for finding its row ("macbookpro"); app-defined
	Kind    string `json:"kind"`    // what sort of work ("export"); app-defined
	State   State  `json:"state"`

	Done  int64  `json:"done"`  // units finished
	Total int64  `json:"total"` // 0 until the work reports a total
	Unit  string `json:"unit"`  // "files"
	Step  int    `json:"step"`  // 1-based; 0 when the work has no steps
	Steps int    `json:"steps"`
	Stage string `json:"stage"` // "Uploading"

	Words string     `json:"words"` // the app's words for the current state
	Stop  StopMode   `json:"stop"`  // how it can be stopped; false when it can't
	Error *ErrorInfo `json:"error"` // set while Failed, or while Waiting for an automatic retry

	Retry   RetryPolicy `json:"retry"`
	NextTry *int64      `json:"nextTry"` // when the next automatic retry runs

	StartedAt *int64 `json:"startedAt"` // when it first ran; null while it has never run
	UpdatedAt int64  `json:"updatedAt"` // the last change of any kind
}

// ChangedPayload is the payload of tasks:changed: every task in the list
// (active ones, and finished ones not yet dismissed), in the order they were
// added. Tasks is never nil.
type ChangedPayload struct {
	Tasks []Snapshot `json:"tasks"`
}

// Progress is what a task reports as it works. Done and Total count Unit;
// Step and Steps count stages, named by Stage.
type Progress struct {
	Done  int64
	Total int64
	Unit  string
	Step  int
	Steps int
	Stage string
}

// Error codes.
const (
	ErrNotFound     errors.Code = "tasks_not_found"
	ErrDuplicateID  errors.Code = "tasks_duplicate_id"
	ErrInvalidSpec  errors.Code = "tasks_invalid_spec"
	ErrInvalidState errors.Code = "tasks_invalid_state"
	ErrUnsupported  errors.Code = "tasks_unsupported"
)

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrNotFound:     "That task is no longer in the list",
		ErrDuplicateID:  "A task with this ID is already in the list",
		ErrInvalidSpec:  "The task isn't set up correctly",
		ErrInvalidState: "The task can't do that in its current state",
		ErrUnsupported:  "This task can't do that",
	})
}

// NoTimeout turns a task's timeout on silence off (Spec.Timeout,
// Task.SetTimeout). Use it only while the task waits on something it has
// named.
const NoTimeout time.Duration = -1

// Defaults, from BEHAVIOUR.md's timings.
const (
	DefaultProgressInterval = 250 * time.Millisecond // four snapshots a second
	DefaultTimeout          = 2 * time.Minute
	DefaultBackoffJitter    = 0.2
	DefaultWakeInterval     = 5 * time.Second
	DefaultWakeThreshold    = 15 * time.Second
)

// DefaultBackoff is the wait before each automatic retry: 2, 4, 8 and 16 s,
// then every 30 s.
var DefaultBackoff = []time.Duration{
	2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second,
}

// Clock is the time source. Inject a fake with WithClock in tests.
type Clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is a pending AfterFunc call.
type Timer interface {
	Stop() bool
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) AfterFunc(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }

// durationWords writes a duration as BEHAVIOUR.md does: "34 s", "3 min",
// "3 h 12 m".
func durationWords(d time.Duration) string {
	switch {
	case d < time.Minute:
		s := int(d.Round(time.Second) / time.Second)
		if s < 1 {
			s = 1
		}
		return fmt.Sprintf("%d s", s)
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d/time.Minute))
	default:
		h := int(d / time.Hour)
		m := int((d % time.Hour) / time.Minute)
		if m == 0 {
			return fmt.Sprintf("%d h", h)
		}
		return fmt.Sprintf("%d h %d m", h, m)
	}
}

func millis(t time.Time) int64 { return t.UnixMilli() }

func millisPtr(t time.Time) *int64 {
	v := t.UnixMilli()
	return &v
}
