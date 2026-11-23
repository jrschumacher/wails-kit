package tasks

import (
	"encoding/json"
	stderrors "errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
)

type harness struct {
	t     *testing.T
	clock *fakeClock
	mem   *events.MemoryEmitter
	em    *events.Emitter
	svc   *Service
}

func newHarness(t *testing.T, opts ...ServiceOption) *harness {
	t.Helper()
	h := &harness{t: t, clock: newFakeClock(), mem: events.NewMemoryEmitter()}
	h.em = events.NewEmitter(h.mem, events.WithHistory(8))
	base := []ServiceOption{
		WithEmitter(h.em),
		WithClock(h.clock),
		WithRandom(func() float64 { return 0.5 }), // no jitter
	}
	h.svc = NewService(append(base, opts...)...)
	t.Cleanup(h.svc.Close)
	return h
}

func (h *harness) add(spec Spec) *Task {
	h.t.Helper()
	task, err := h.svc.Add(spec)
	if err != nil {
		h.t.Fatalf("Add: %v", err)
	}
	return task
}

// snapshots returns every tasks:changed payload broadcast so far.
func (h *harness) snapshots() []ChangedPayload {
	h.t.Helper()
	var out []ChangedPayload
	for _, r := range h.mem.Broadcasts() {
		if r.Name != EventChanged {
			h.t.Fatalf("unexpected event %q", r.Name)
		}
		out = append(out, r.Data.(ChangedPayload))
	}
	return out
}

// last returns the snapshot of id in the latest tasks:changed.
func (h *harness) last(id string) Snapshot {
	h.t.Helper()
	all := h.snapshots()
	if len(all) == 0 {
		h.t.Fatal("no tasks:changed emitted")
	}
	for _, s := range all[len(all)-1].Tasks {
		if s.ID == id {
			return s
		}
	}
	h.t.Fatalf("task %q not in the last snapshot", id)
	return Snapshot{}
}

func (h *harness) count() int { return len(h.snapshots()) }

func TestAddEmitsSnapshot(t *testing.T) {
	h := newHarness(t)
	h.add(Spec{ID: "export", Subject: "report.pdf", Kind: "export", Words: "Exporting", Stop: StopCancel})

	if h.count() != 1 {
		t.Fatalf("expected 1 snapshot, got %d", h.count())
	}
	s := h.last("export")
	if s.State != StateRunning || s.Words != "Exporting" || s.Retry != RetryManual || s.Stop != StopCancel {
		t.Errorf("unexpected snapshot: %+v", s)
	}
	if s.StartedAt == nil || *s.StartedAt != h.clock.Now().UnixMilli() || s.UpdatedAt != *s.StartedAt {
		t.Errorf("unexpected times: startedAt %v updatedAt %d", s.StartedAt, s.UpdatedAt)
	}
}

func TestSnapshotJSONShape(t *testing.T) {
	h := newHarness(t)
	h.add(Spec{ID: "a", Queued: true})
	b, err := json.Marshal(h.snapshots()[0])
	if err != nil {
		t.Fatal(err)
	}
	var got map[string][]map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	task := got["tasks"][0]
	want := []string{"id", "parent", "subject", "kind", "state", "done", "total", "unit",
		"step", "steps", "stage", "words", "stop", "error", "retry", "nextTry", "startedAt", "updatedAt"}
	if len(task) != len(want) {
		t.Errorf("expected %d keys, got %d: %v", len(want), len(task), task)
	}
	for _, k := range want {
		if _, ok := task[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
	if task["stop"] != false {
		t.Errorf("stop: want false, got %v", task["stop"])
	}
	for _, k := range []string{"error", "nextTry", "startedAt"} {
		if task[k] != nil {
			t.Errorf("%s: want null, got %v", k, task[k])
		}
	}
	if task["state"] != "queued" {
		t.Errorf("state: want queued, got %v", task["state"])
	}

	empty, _ := json.Marshal(ChangedPayload{Tasks: NewService().Tasks()})
	if string(empty) != `{"tasks":[]}` {
		t.Errorf("empty list: got %s", empty)
	}
}

func TestStopModeJSON(t *testing.T) {
	for _, m := range []StopMode{StopNone, StopCancel, StopStop, StopStopAsks, StopPause, StopUnsafe} {
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		var back StopMode
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		if back != m {
			t.Errorf("%q round-tripped to %q via %s", m, back, b)
		}
	}
}

func TestStateChangesGoOutAtOnce(t *testing.T) {
	h := newHarness(t)
	task := h.add(Spec{ID: "a", Queued: true})
	task.Running("Copying")
	task.Waiting("Waiting for GitHub…")
	task.Paused("Paused · 12 of 40 files")
	task.Running("")
	task.Stalled("GitHub sign-in expired")
	task.Running("Copying")
	task.Done("Copied")

	var states []State
	for _, p := range h.snapshots() {
		states = append(states, p.Tasks[0].State)
	}
	want := []State{StateQueued, StateRunning, StateWaiting, StatePaused, StateRunning, StateStalled, StateRunning, StateDone}
	if len(states) != len(want) {
		t.Fatalf("states %v, want %v", states, want)
	}
	for i := range want {
		if states[i] != want[i] {
			t.Fatalf("states %v, want %v", states, want)
		}
	}
	if h.last("a").Words != "Copied" {
		t.Errorf("words: %q", h.last("a").Words)
	}
}

func TestProgressIsCoalesced(t *testing.T) {
	h := newHarness(t)
	task := h.add(Spec{ID: "a"}) // snapshot 1, at t=0

	// 100 reports over one second, one every 10 ms.
	for i := 1; i <= 100; i++ {
		task.Progress(Progress{Done: int64(i), Total: 100, Unit: "files"})
		h.clock.Advance(10 * time.Millisecond)
	}
	progressSnapshots := h.count() - 1
	if progressSnapshots > 4 {
		t.Errorf("expected at most 4 progress snapshots in 1 s, got %d", progressSnapshots)
	}
	if progressSnapshots < 3 {
		t.Errorf("expected progress to keep flowing, got %d snapshots", progressSnapshots)
	}

	// The last report always goes out.
	h.clock.Advance(time.Second)
	if got := h.last("a").Done; got != 100 {
		t.Errorf("last snapshot done = %d, want 100", got)
	}
}

func TestDoneIsNeverDroppedAfterProgress(t *testing.T) {
	h := newHarness(t)
	task := h.add(Spec{ID: "a"})
	task.Progress(Progress{Done: 39, Total: 40}) // held back: within 250 ms of Add
	n := h.count()
	task.Done("Copied")
	if h.count() != n+1 {
		t.Fatalf("Done should go out at once")
	}
	s := h.last("a")
	if s.State != StateDone || s.Done != 39 {
		t.Errorf("unexpected final snapshot: %+v", s)
	}
	// The held-back progress snapshot was folded into Done's.
	h.clock.Advance(time.Second)
	if h.count() != n+1 {
		t.Errorf("expected no snapshot after Done, got %d more", h.count()-n-1)
	}
}

func TestReplayAllSendsCurrentList(t *testing.T) {
	h := newHarness(t)
	a := h.add(Spec{ID: "a"})
	h.add(Spec{ID: "b"})
	for i := 0; i < 20; i++ {
		a.Progress(Progress{Done: int64(i), Total: 20})
		h.clock.Advance(300 * time.Millisecond)
	}
	a.Done("Copied")

	h.em.RegisterWindow("reloaded", h.mem.MemoryWindow("reloaded"))
	h.em.ReplayAll("reloaded")
	got := h.mem.EventsFor("reloaded")
	if len(got) != 1 || got[0].Name != EventChanged {
		t.Fatalf("expected one replayed tasks:changed, got %+v", got)
	}
	list := got[0].Data.(ChangedPayload).Tasks
	if len(list) != 2 || list[0].State != StateDone || list[1].State != StateRunning {
		t.Errorf("replayed list is not current: %+v", list)
	}
}

func TestFailUsesRegisteredMessage(t *testing.T) {
	const code errors.Code = "tasks_test_export"
	errors.RegisterMessages(map[errors.Code]string{code: "Couldn't export the report"})

	h := newHarness(t)
	task := h.add(Spec{ID: "a"})
	task.Fail(errors.New(code, "write /tmp/x: disk full", nil), "Couldn't export")

	s := h.last("a")
	if s.State != StateFailed || s.Words != "Couldn't export" {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
	if s.Error == nil || s.Error.Code != code || s.Error.UserMsg != "Couldn't export the report" ||
		s.Error.Message != "write /tmp/x: disk full" {
		t.Errorf("unexpected error: %+v", s.Error)
	}

	other := h.add(Spec{ID: "b"})
	other.Fail(stderrors.New("boom"), "Couldn't export")
	if e := h.last("b").Error; e.Code != errors.ErrInternal || e.UserMsg != errors.GetUserMessage(nil) || e.Message != "boom" {
		t.Errorf("plain error: %+v", e)
	}
}

func TestTimeoutOnSilence(t *testing.T) {
	h := newHarness(t)
	task := h.add(Spec{ID: "a"})
	ctx := task.Context()

	h.clock.Advance(119 * time.Second)
	task.Progress(Progress{Done: 1, Total: 2}) // a sign of life restarts the clock
	h.clock.Advance(119 * time.Second)
	if h.last("a").State != StateRunning {
		t.Fatal("timed out despite progress")
	}
	h.clock.Advance(time.Second)
	s := h.last("a")
	if s.State != StateFailed || s.Words != "Timed out after 2 min" || s.Error == nil || s.Error.Code != errors.ErrTimeout {
		t.Fatalf("unexpected snapshot after timeout: %+v %+v", s, s.Error)
	}
	if ctx.Err() == nil {
		t.Error("context not cancelled on timeout")
	}

	// Late reports from the abandoned work are ignored.
	task.Done("Copied")
	task.Progress(Progress{Done: 2, Total: 2})
	if h.last("a").State != StateFailed {
		t.Error("a late report revived a timed-out task")
	}
}

func TestNoTimeoutWhileQueuedPausedStalledOrOff(t *testing.T) {
	h := newHarness(t)
	h.add(Spec{ID: "queued", Queued: true})
	paused := h.add(Spec{ID: "paused"})
	paused.Paused("Paused")
	stalled := h.add(Spec{ID: "stalled"})
	stalled.Stalled("Sign in")
	h.add(Spec{ID: "off", Timeout: NoTimeout})
	named := h.add(Spec{ID: "named"})
	named.Waiting("Waiting for the job on GitHub")
	named.SetTimeout(NoTimeout)
	waiting := h.add(Spec{ID: "waiting"})
	waiting.Waiting("Waiting for GitHub…")
	h.add(Spec{ID: "custom", Timeout: 30 * time.Second})

	h.clock.Advance(10 * time.Minute)
	for _, id := range []string{"queued", "paused", "stalled", "off", "named"} {
		if s := h.last(id); s.State.Finished() {
			t.Errorf("%s timed out: %+v", id, s)
		}
	}
	if s := h.last("waiting"); s.State != StateFailed {
		t.Errorf("a waiting task keeps its timeout unless turned off, got %s", s.State)
	}
	if s := h.last("custom"); s.State != StateFailed || s.Words != "Timed out after 30 s" {
		t.Errorf("custom timeout: %+v", s)
	}
}

func TestServiceTimeoutOption(t *testing.T) {
	h := newHarness(t, WithTimeout(NoTimeout))
	h.add(Spec{ID: "a"})
	h.add(Spec{ID: "b", Timeout: time.Minute})
	h.clock.Advance(5 * time.Minute)
	if h.last("a").State != StateRunning {
		t.Error("WithTimeout(NoTimeout) should turn the default off")
	}
	if h.last("b").State != StateFailed {
		t.Error("a task's own timeout still applies")
	}
}

func TestAutoRetryWithBackoff(t *testing.T) {
	h := newHarness(t)
	var retries atomic.Int32
	task := h.add(Spec{ID: "sync", Words: "Syncing", Retry: RetryAuto, OnRetry: func(*Task) {
		retries.Add(1)
	}})
	firstCtx := task.Context()

	wantWaits := []time.Duration{2, 4, 8, 16, 30, 30}
	for i, w := range wantWaits {
		start := h.clock.Now()
		task.Fail(stderrors.New("connection refused"), "Can't reach the server")
		s := h.last("sync")
		if s.State != StateWaiting || s.Error == nil || s.NextTry == nil {
			t.Fatalf("attempt %d: expected waiting with error and nextTry, got %+v", i+1, s)
		}
		if got := time.Duration(*s.NextTry-start.UnixMilli()) * time.Millisecond; got != w*time.Second {
			t.Fatalf("attempt %d: wait %s, want %s", i+1, got, w*time.Second)
		}
		h.clock.Advance(w*time.Second - time.Millisecond)
		if int(retries.Load()) != i {
			t.Fatalf("attempt %d: retried early", i+1)
		}
		h.clock.Advance(time.Millisecond)
		if int(retries.Load()) != i+1 {
			t.Fatalf("attempt %d: not retried after %s", i+1, w*time.Second)
		}
		s = h.last("sync")
		if s.State != StateRunning || s.Error != nil || s.NextTry != nil || s.Words != "Syncing" {
			t.Fatalf("attempt %d: retry should start a clean attempt, got %+v", i+1, s)
		}
	}
	if firstCtx.Err() == nil {
		t.Error("the first attempt's context should be cancelled")
	}
	if task.Context().Err() != nil {
		t.Error("the current attempt's context should be live")
	}
}

func TestBackoffJitter(t *testing.T) {
	for _, tc := range []struct {
		r    float64
		want time.Duration
	}{
		{0, 1600 * time.Millisecond},
		{0.5, 2 * time.Second},
		{0.75, 2200 * time.Millisecond},
	} {
		s := NewService(WithRandom(func() float64 { return tc.r }))
		if got := s.backoffFor(1); got != tc.want {
			t.Errorf("r=%v: got %s, want %s", tc.r, got, tc.want)
		}
	}
	s := NewService(WithRandom(func() float64 { return 0.9999 }))
	if got := s.backoffFor(9); got < 35*time.Second || got > 36*time.Second {
		t.Errorf("max jitter on 30 s: %s", got)
	}
}

func TestRetryNowCarriesBackoffOn(t *testing.T) {
	h := newHarness(t)
	task := h.add(Spec{ID: "a", Retry: RetryAuto, OnRetry: func(*Task) {}})
	task.Fail(stderrors.New("x"), "Can't reach the server") // waits 2 s
	if err := h.svc.Retry("a"); err != nil {
		t.Fatal(err)
	}
	if h.last("a").State != StateRunning {
		t.Fatal("Retry now should run at once")
	}
	start := h.clock.Now()
	task.Fail(stderrors.New("x"), "Can't reach the server")
	if got := *h.last("a").NextTry - start.UnixMilli(); got != 4000 {
		t.Errorf("backoff should carry on (4 s), got %d ms", got)
	}
	// The old timer must not fire a second retry.
	h.clock.Advance(2 * time.Second)
	if h.last("a").State != StateWaiting {
		t.Error("a stale retry timer fired")
	}
}

func TestManualAndUnsafeWaitForThePerson(t *testing.T) {
	for _, policy := range []RetryPolicy{RetryManual, RetryUnsafe} {
		h := newHarness(t)
		var retries int
		task := h.add(Spec{ID: "a", Retry: policy, OnRetry: func(*Task) { retries++ }})
		task.Fail(stderrors.New("x"), "Couldn't confirm the comment was posted")
		h.clock.Advance(time.Hour)
		s := h.last("a")
		if s.State != StateFailed || s.NextTry != nil || retries != 0 {
			t.Fatalf("%s: expected failed with no retry, got %+v (%d retries)", policy, s, retries)
		}
		if err := h.svc.Retry("a"); err != nil {
			t.Fatalf("%s: %v", policy, err)
		}
		if retries != 1 || h.last("a").State != StateRunning {
			t.Errorf("%s: Retry should run OnRetry", policy)
		}
	}
}

func TestMaxAttempts(t *testing.T) {
	h := newHarness(t)
	task := h.add(Spec{ID: "a", Retry: RetryAuto, MaxAttempts: 2, OnRetry: func(*Task) {}})
	task.Fail(stderrors.New("x"), "Can't reach the server")
	h.clock.Advance(2 * time.Second)
	task.Fail(stderrors.New("x"), "Can't reach the server")
	if s := h.last("a"); s.State != StateFailed || s.NextTry != nil {
		t.Errorf("expected failed after 2 attempts, got %+v", s)
	}
}

func TestAutoWithoutOnRetryFails(t *testing.T) {
	h := newHarness(t)
	task := h.add(Spec{ID: "a", Retry: RetryAuto})
	task.Fail(stderrors.New("x"), "Can't reach the server")
	if h.last("a").State != StateFailed {
		t.Error("a task that can't be retried should fail")
	}
	if err := h.svc.Retry("a"); !errors.IsCode(err, ErrUnsupported) {
		t.Errorf("Retry: %v", err)
	}
}

func TestRetryErrors(t *testing.T) {
	h := newHarness(t)
	h.add(Spec{ID: "a", OnRetry: func(*Task) {}})
	if err := h.svc.Retry("a"); !errors.IsCode(err, ErrInvalidState) {
		t.Errorf("retrying a running task: %v", err)
	}
	if err := h.svc.Retry("nope"); !errors.IsCode(err, ErrNotFound) {
		t.Errorf("retrying a missing task: %v", err)
	}
}

func TestSleepAndWakeRestartClocks(t *testing.T) {
	h := newHarness(t, WithWakeDetection(0, 0))
	h.add(Spec{ID: "a"})
	h.clock.Advance(90 * time.Second)
	h.svc.Sleep()
	h.clock.Advance(time.Hour) // asleep: nothing fires
	if h.last("a").State != StateRunning {
		t.Fatal("a clock ran while asleep")
	}
	h.svc.Wake()
	h.clock.Advance(119 * time.Second)
	if h.last("a").State != StateRunning {
		t.Fatal("the timeout didn't start again from zero on wake")
	}
	h.clock.Advance(time.Second)
	if h.last("a").State != StateFailed {
		t.Fatal("the timeout should run again after wake")
	}
}

func TestWakeRetriesAtOnceAndDropsBackoff(t *testing.T) {
	h := newHarness(t, WithWakeDetection(0, 0))
	var retries int
	task := h.add(Spec{ID: "a", Retry: RetryAuto, OnRetry: func(*Task) { retries++ }})
	task.Fail(stderrors.New("x"), "Can't reach the server")
	h.clock.Advance(2 * time.Second)
	task.Fail(stderrors.New("x"), "Can't reach the server") // now waiting 4 s
	h.svc.Sleep()
	h.svc.Wake()
	if retries != 2 || h.last("a").State != StateRunning {
		t.Fatalf("wake should retry at once: %d retries, %+v", retries, h.last("a"))
	}
	start := h.clock.Now()
	task.Fail(stderrors.New("x"), "Can't reach the server")
	if got := *h.last("a").NextTry - start.UnixMilli(); got != 2000 {
		t.Errorf("wake should drop the backoff (2 s), got %d ms", got)
	}
}

func TestWakeDetectedFromClockJump(t *testing.T) {
	h := newHarness(t)
	var retries int
	h.add(Spec{ID: "run"})
	task := h.add(Spec{ID: "sync", Retry: RetryAuto, OnRetry: func(*Task) { retries++ }})
	task.Fail(stderrors.New("x"), "Can't reach the server")
	h.clock.Advance(2 * time.Second) // retry 1
	task.Fail(stderrors.New("x"), "Can't reach the server")

	h.clock.Advance(100 * time.Second) // 100 s of the run task's 120 s used
	// Asleep for an hour: timers don't see it, the wall clock does.
	h.clock.Suspend(time.Hour)
	h.clock.Advance(5 * time.Second) // the detector's next tick
	if retries != 2 {
		t.Errorf("expected a retry at once on wake, got %d retries", retries)
	}
	h.clock.Advance(110 * time.Second)
	if h.last("run").State != StateRunning {
		t.Error("the run task's timeout should start again from zero on wake")
	}

	// No false wake while the machine is awake.
	h2 := newHarness(t)
	h2.add(Spec{ID: "a", Timeout: NoTimeout})
	n := h2.count()
	h2.clock.Advance(time.Hour)
	if h2.count() != n {
		t.Error("the detector fired without a clock jump")
	}
}

func TestStop(t *testing.T) {
	h := newHarness(t)

	var stopped *Task
	h.add(Spec{ID: "with", Stop: StopStop, OnStop: func(t *Task) {
		stopped = t
		t.Stopped("12 of 40 files copied")
	}})
	if err := h.svc.Stop("with"); err != nil {
		t.Fatal(err)
	}
	if stopped == nil || h.last("with").State != StateStopped || h.last("with").Words != "12 of 40 files copied" {
		t.Errorf("OnStop: %+v", h.last("with"))
	}

	without := h.add(Spec{ID: "without", Stop: StopCancel})
	ctx := without.Context()
	if err := h.svc.Stop("without"); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Error("Stop without OnStop should cancel the context")
	}
	without.Cancelled()
	if h.last("without").State != StateCancelled {
		t.Error("expected cancelled")
	}

	h.add(Spec{ID: "none"})
	if err := h.svc.Stop("none"); !errors.IsCode(err, ErrUnsupported) {
		t.Errorf("stopping an unstoppable task: %v", err)
	}
	if err := h.svc.Stop("with"); !errors.IsCode(err, ErrInvalidState) {
		t.Errorf("stopping a finished task: %v", err)
	}

	retrying := h.add(Spec{ID: "retrying", Stop: StopCancel, Retry: RetryAuto, OnRetry: func(*Task) {
		t.Error("a cancelled task was retried")
	}})
	retrying.Fail(stderrors.New("x"), "Can't reach the server")
	if err := h.svc.Stop("retrying"); err != nil {
		t.Fatal(err)
	}
	if h.last("retrying").State != StateCancelled {
		t.Error("stopping a task waiting to retry should cancel it at once")
	}
	h.clock.Advance(time.Minute)
}

func TestPauseAndResume(t *testing.T) {
	h := newHarness(t)
	h.add(Spec{ID: "a", Stop: StopPause,
		OnPause:  func(t *Task) { t.Paused("Paused · 12 of 40 files") },
		OnResume: func(t *Task) { t.Running("") },
	})
	if err := h.svc.Pause("a"); err != nil {
		t.Fatal(err)
	}
	if h.last("a").State != StatePaused {
		t.Fatal("expected paused")
	}
	if err := h.svc.Resume("a"); err != nil {
		t.Fatal(err)
	}
	if h.last("a").State != StateRunning {
		t.Fatal("expected running")
	}
	h.add(Spec{ID: "b"})
	if err := h.svc.Pause("b"); !errors.IsCode(err, ErrUnsupported) {
		t.Errorf("pausing without OnPause: %v", err)
	}
}

func TestPartsAndDismiss(t *testing.T) {
	h := newHarness(t)
	parent := h.add(Spec{ID: "review", Words: "Running reviewers"})
	var parts []*Task
	for _, id := range []string{"r1", "r2"} {
		p, err := parent.AddPart(Spec{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, p)
	}
	if h.last("r1").Parent != "review" {
		t.Error("part should name its parent")
	}
	parts[0].Done("")
	parts[1].Fail(stderrors.New("x"), "Couldn't run")
	parent.Fail(stderrors.New("x"), "1 of 2 reviewers finished · 1 failed")

	extra := h.add(Spec{ID: "other"})
	if err := h.svc.Dismiss("other"); !errors.IsCode(err, ErrInvalidState) {
		t.Errorf("dismissing a running task: %v", err)
	}
	if err := h.svc.Dismiss("review"); err != nil {
		t.Fatal(err)
	}
	list := h.snapshots()[h.count()-1].Tasks
	if len(list) != 1 || list[0].ID != "other" {
		t.Errorf("dismiss should remove the task and its parts: %+v", list)
	}
	// Reports through a dismissed handle are ignored.
	parts[0].Running("")
	if len(h.svc.Tasks()) != 1 {
		t.Error("a dismissed part came back")
	}
	if err := h.svc.Dismiss("review"); !errors.IsCode(err, ErrNotFound) {
		t.Errorf("dismissing twice: %v", err)
	}

	extra.Done("")
	h.add(Spec{ID: "running"})
	if n := h.svc.DismissFinished(); n != 1 {
		t.Errorf("DismissFinished removed %d, want 1", n)
	}
	if list := h.svc.Tasks(); len(list) != 1 || list[0].ID != "running" {
		t.Errorf("unexpected list: %+v", list)
	}
}

func TestAddValidation(t *testing.T) {
	h := newHarness(t)
	h.add(Spec{ID: "a"})
	for name, tc := range map[string]struct {
		spec Spec
		code errors.Code
	}{
		"duplicate":      {Spec{ID: "a"}, ErrDuplicateID},
		"missing parent": {Spec{Parent: "nope"}, ErrNotFound},
		"bad stop":       {Spec{Stop: "halt"}, ErrInvalidSpec},
		"bad retry":      {Spec{Retry: "always"}, ErrInvalidSpec},
		"bad attempts":   {Spec{MaxAttempts: -1}, ErrInvalidSpec},
	} {
		if _, err := h.svc.Add(tc.spec); !errors.IsCode(err, tc.code) {
			t.Errorf("%s: got %v, want %s", name, err, tc.code)
		}
	}
	b := h.add(Spec{})
	c := h.add(Spec{})
	if b.ID() == "" || b.ID() == c.ID() {
		t.Errorf("generated IDs: %q %q", b.ID(), c.ID())
	}
}

func TestHandlerMayCallBack(t *testing.T) {
	h := newHarness(t)
	var task *Task
	var seen atomic.Int32
	events.On(h.em, EventChanged, func(p ChangedPayload) {
		seen.Add(1)
		_ = h.svc.Tasks()
		if len(p.Tasks) == 1 && p.Tasks[0].State == StateRunning && seen.Load() == 2 {
			task.Done("Done from a handler")
		}
	})
	task = h.add(Spec{ID: "a", Queued: true})
	task.Running("")
	if s := h.last("a"); s.State != StateDone {
		t.Errorf("the handler's change should go out after it: %+v", s)
	}
}

func TestWithoutEmitter(t *testing.T) {
	s := NewService(WithClock(newFakeClock()))
	defer s.Close()
	task, err := s.Add(Spec{ID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	task.Progress(Progress{Done: 1})
	task.Done("")
	if got, _ := s.Get("a"); got.State != StateDone || got.Done != 1 {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestCloseFlushesAndCancels(t *testing.T) {
	h := newHarness(t)
	task := h.add(Spec{ID: "a"})
	task.Progress(Progress{Done: 7, Total: 10})
	ctx := task.Context()
	h.svc.Close()
	if h.last("a").Done != 7 {
		t.Error("Close should send the held-back snapshot")
	}
	if ctx.Err() == nil {
		t.Error("Close should cancel task contexts")
	}
	if _, err := h.svc.Add(Spec{}); !errors.IsCode(err, ErrInvalidState) {
		t.Errorf("Add after Close: %v", err)
	}
}

func TestDurationWords(t *testing.T) {
	for d, want := range map[time.Duration]string{
		34 * time.Second:             "34 s",
		200 * time.Millisecond:       "1 s",
		2 * time.Minute:              "2 min",
		3*time.Hour + 12*time.Minute: "3 h 12 m",
		time.Hour:                    "1 h",
		90 * time.Second:             "1 min",
	} {
		if got := durationWords(d); got != want {
			t.Errorf("%s: got %q, want %q", d, got, want)
		}
	}
}

func TestStates(t *testing.T) {
	var finished []string
	for _, s := range States {
		if s.Finished() {
			finished = append(finished, string(s))
		}
	}
	if strings.Join(finished, ",") != "done,failed,cancelled,stopped" || len(States) != 9 {
		t.Errorf("states: %v, finished %v", States, finished)
	}
}

func TestRealClock(t *testing.T) {
	mem := events.NewMemoryEmitter()
	s := NewService(WithEmitter(events.NewEmitter(mem)), WithTimeout(20*time.Millisecond))
	defer s.Close()
	task, err := s.Add(Spec{ID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-task.Context().Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the real clock's timeout never fired")
	}
	if got, _ := s.Get("a"); got.State != StateFailed {
		t.Errorf("expected failed, got %s", got.State)
	}
}
