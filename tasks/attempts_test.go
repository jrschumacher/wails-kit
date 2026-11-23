package tasks

import (
	stderrors "errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/errors"
)

// A task waiting for an automatic retry ignores late reports from the
// attempt that failed: they must not finish it or change its words.
func TestReportsDuringRetryWaitAreIgnored(t *testing.T) {
	for name, report := range map[string]func(*Task){
		"Done":      func(t *Task) { t.Done("Synced") },
		"Stopped":   func(t *Task) { t.Stopped("3 of 9 synced") },
		"Cancelled": func(t *Task) { t.Cancelled() },
		"Fail":      func(t *Task) { t.Fail(stderrors.New("again"), "Still can't reach it") },
		"SetWords":  func(t *Task) { t.SetWords("Syncing 3 of 9") },
		"Progress":  func(t *Task) { t.Progress(Progress{Done: 3, Total: 9}) },
		"Running":   func(t *Task) { t.Running("Syncing") },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			var retries int
			task := h.add(Spec{ID: "a", Retry: RetryAuto, OnRetry: func(*Task) { retries++ }})
			task.Fail(stderrors.New("x"), "Can't reach the server")
			before := h.last("a")

			report(task)

			s := h.last("a")
			if s.State != StateWaiting || s.Words != before.Words || s.NextTry == nil || *s.NextTry != *before.NextTry {
				t.Fatalf("a late %s changed the waiting task: %+v", name, s)
			}
			h.clock.Advance(2 * time.Second)
			if retries != 1 {
				t.Errorf("the retry was lost after a late %s", name)
			}
		})
	}
}

// The README's pattern: the work stops when its context ends and reports
// Stopped. After a timeout on an auto task, that report must not cost the retry.
func TestAbandonedAttemptAfterTimeoutKeepsRetry(t *testing.T) {
	h := newHarness(t)
	var retries int
	task := h.add(Spec{ID: "a", Retry: RetryAuto, OnRetry: func(*Task) { retries++ }})
	ctx := task.Context()
	h.clock.Advance(2 * time.Minute) // timeout: waits to retry
	if ctx.Err() == nil || h.last("a").State != StateWaiting {
		t.Fatalf("expected a retry wait after the timeout: %+v", h.last("a"))
	}
	task.Stopped("0 of 9 synced") // the abandoned work notices its context
	h.clock.Advance(2 * time.Second)
	if retries != 1 || h.last("a").State != StateRunning {
		t.Errorf("the abandoned attempt's Stopped lost the retry: %+v", h.last("a"))
	}
}

// Each attempt reports through its own handle; a stale handle is ignored.
func TestStaleAttemptCannotTouchTheRetry(t *testing.T) {
	h := newHarness(t)
	var second *Task
	first := h.add(Spec{ID: "a", OnRetry: func(t *Task) { second = t }})
	firstCtx := first.Context()
	first.Fail(stderrors.New("x"), "Couldn't sync")
	if err := h.svc.Retry("a"); err != nil {
		t.Fatal(err)
	}
	if second == nil || second == first {
		t.Fatal("OnRetry should get a new handle for the new attempt")
	}
	if firstCtx.Err() == nil || second.Context().Err() != nil {
		t.Fatal("each attempt should have its own context")
	}

	// The first attempt's work ends late.
	first.Fail(firstCtx.Err(), "Couldn't sync")
	first.Done("Synced")
	first.Progress(Progress{Done: 1})
	if s := h.last("a"); s.State != StateRunning || s.Done != 0 {
		t.Fatalf("a stale handle changed the new attempt: %+v", s)
	}

	second.Done("Synced")
	if h.last("a").State != StateDone {
		t.Error("the current handle should report")
	}
	if first.ID() != "a" || second.ID() != "a" {
		t.Error("handles share the task's ID")
	}
}

// Cancel takes a queued task off the queue, even without an OnStop.
func TestStopQueuedWithoutOnStopCancels(t *testing.T) {
	h := newHarness(t)
	task := h.add(Spec{ID: "a", Queued: true, Stop: StopCancel})
	if err := h.svc.Stop("a"); err != nil {
		t.Fatal(err)
	}
	if h.last("a").State != StateCancelled || task.Context().Err() == nil {
		t.Errorf("expected cancelled at once: %+v", h.last("a"))
	}
}

// Wake drops the backoff but doesn't reset the attempt count.
func TestWakeKeepsMaxAttempts(t *testing.T) {
	h := newHarness(t, WithWakeDetection(0, 0))
	var retries int
	var current *Task
	task := h.add(Spec{ID: "a", Retry: RetryAuto, MaxAttempts: 2, OnRetry: func(t *Task) { retries++; current = t }})
	task.Fail(stderrors.New("x"), "Can't reach the server") // attempt 1 failed
	h.svc.Wake()                                            // retries at once: attempt 2
	if retries != 1 {
		t.Fatalf("wake should retry at once, got %d", retries)
	}
	current.Fail(stderrors.New("x"), "Can't reach the server")
	if s := h.last("a"); s.State != StateFailed {
		t.Errorf("MaxAttempts 2 should fail after the second attempt, got %+v", s)
	}
}

// SetTimeout(NoTimeout) lasts only while the task waits.
func TestNoTimeoutEndsWhenWaitingEnds(t *testing.T) {
	h := newHarness(t)
	task := h.add(Spec{ID: "a"})
	task.Waiting("Waiting for the job on GitHub")
	task.SetTimeout(NoTimeout)
	h.clock.Advance(10 * time.Minute)
	if h.last("a").State != StateWaiting {
		t.Fatal("no timeout while waiting on something named")
	}
	task.Running("Downloading results")
	h.clock.Advance(2 * time.Minute)
	if s := h.last("a"); s.State != StateFailed {
		t.Errorf("the default timeout should be back once running: %+v", s)
	}
}

// Pause and Resume only reach the app from a state they make sense in.
func TestPauseResumeNeedAValidState(t *testing.T) {
	h := newHarness(t)
	var pauses, resumes int
	h.add(Spec{ID: "a", Stop: StopPause,
		OnPause:  func(t *Task) { pauses++; t.Paused("Paused") },
		OnResume: func(t *Task) { resumes++; t.Running("") },
	})
	if err := h.svc.Resume("a"); !errors.IsCode(err, ErrInvalidState) || resumes != 0 {
		t.Errorf("resuming a running task: %v (%d calls)", err, resumes)
	}
	if err := h.svc.Pause("a"); err != nil || pauses != 1 {
		t.Fatalf("pause: %v", err)
	}
	if err := h.svc.Pause("a"); !errors.IsCode(err, ErrInvalidState) || pauses != 1 {
		t.Errorf("pausing a paused task: %v (%d calls)", err, pauses)
	}
	h.add(Spec{ID: "q", Queued: true, OnPause: func(*Task) { pauses++ }})
	if err := h.svc.Pause("q"); !errors.IsCode(err, ErrInvalidState) || pauses != 1 {
		t.Errorf("pausing a queued task: %v", err)
	}
}

// Stop during a retry wait follows the task's stop mode.
func TestStopDuringRetryWaitFollowsMode(t *testing.T) {
	for mode, want := range map[StopMode]State{
		StopCancel:   StateCancelled,
		StopStop:     StateStopped,
		StopStopAsks: StateStopped,
	} {
		h := newHarness(t)
		task := h.add(Spec{ID: "a", Stop: mode, Retry: RetryAuto, OnRetry: func(*Task) {}})
		task.Fail(stderrors.New("x"), "Can't reach the server")
		if err := h.svc.Stop("a"); err != nil {
			t.Fatal(err)
		}
		if got := h.last("a").State; got != want {
			t.Errorf("%s: got %s, want %s", mode, got, want)
		}
	}
}

func TestDurationWordsRound(t *testing.T) {
	for d, want := range map[time.Duration]string{
		90 * time.Second:                              "2 min",
		89 * time.Second:                              "1 min",
		59*time.Minute + 40*time.Second:               "1 h",
		3*time.Hour + 11*time.Minute + 40*time.Second: "3 h 12 m",
	} {
		if got := durationWords(d); got != want {
			t.Errorf("%s: got %q, want %q", d, got, want)
		}
	}
}

// The service moves a running task to Waiting after 30 s without a report,
// and back to Running on the next report.
func TestWaitingAfterQuietSpell(t *testing.T) {
	h := newHarness(t)
	task := h.add(Spec{ID: "a", Words: "Fetching runs", WaitingWords: "Waiting for GitHub…"})

	h.clock.Advance(29 * time.Second)
	task.Progress(Progress{Done: 1, Total: 4}) // a report restarts the clock
	h.clock.Advance(29 * time.Second)
	if h.last("a").State != StateRunning {
		t.Fatal("switched to Waiting despite a report")
	}
	n := h.count()
	h.clock.Advance(time.Second)
	s := h.last("a")
	if s.State != StateWaiting || s.Words != "Waiting for GitHub…" || h.count() != n+1 {
		t.Fatalf("expected Waiting at once after 30 s of quiet: %+v", s)
	}

	// The timeout keeps counting from the last report, not from the switch.
	h.clock.Advance(89 * time.Second) // 119 s since the last report
	if h.last("a").State != StateWaiting {
		t.Fatal("timed out early")
	}
	task.SetWords("Fetching runs")
	if s := h.last("a"); s.State != StateRunning {
		t.Fatalf("a report should bring it back to Running: %+v", s)
	}
	h.clock.Advance(30 * time.Second)
	task.Progress(Progress{Done: 2, Total: 4})
	if h.last("a").State != StateRunning {
		t.Fatal("progress should bring it back to Running")
	}
	h.clock.Advance(2 * time.Minute)
	if s := h.last("a"); s.State != StateFailed {
		t.Errorf("still times out 2 min after the last report: %+v", s)
	}
}

// A task the app put in Waiting stays there on SetWords, and the quiet
// switch never runs while the app said Waiting, queued, paused or stalled.
func TestWaitingAfterOnlyFromRunning(t *testing.T) {
	h := newHarness(t, WithTimeout(NoTimeout))
	named := h.add(Spec{ID: "named"})
	named.Waiting("Waiting for the job on GitHub")
	named.SetWords("Waiting for the job on GitHub · 2 ahead")
	h.add(Spec{ID: "queued", Queued: true})
	paused := h.add(Spec{ID: "paused"})
	paused.Paused("Paused")
	h.clock.Advance(time.Minute)
	if s := h.last("named"); s.State != StateWaiting {
		t.Errorf("SetWords moved an app-set Waiting: %+v", s)
	}
	for _, id := range []string{"queued", "paused"} {
		if s := h.last(id); s.State == StateWaiting {
			t.Errorf("%s switched to Waiting", id)
		}
	}

	off := newHarness(t, WithWaitingAfter(NoTimeout))
	off.add(Spec{ID: "a"})
	off.clock.Advance(time.Minute)
	if off.last("a").State != StateRunning {
		t.Error("WithWaitingAfter(NoTimeout) should turn the switch off")
	}
}

func TestTimeoutWordsOption(t *testing.T) {
	h := newHarness(t, WithTimeoutWords(func(d time.Duration) string {
		return "Délai dépassé après " + durationWords(d)
	}))
	h.add(Spec{ID: "a"})
	h.clock.Advance(2 * time.Minute)
	if got := h.last("a").Words; got != "Délai dépassé après 2 min" {
		t.Errorf("timeout words: %q", got)
	}
}

// Bindings exposes only what the frontend may call.
func TestBindingsSurface(t *testing.T) {
	want := []string{"Dismiss", "DismissFinished", "Get", "Pause", "Resume", "Retry", "Stop", "Tasks"}
	typ := reflect.TypeOf(&Bindings{})
	var got []string
	for i := range typ.NumMethod() {
		got = append(got, typ.Method(i).Name)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Bindings methods: %v, want %v", got, want)
	}

	h := newHarness(t)
	b := h.svc.Bindings()
	task := h.add(Spec{ID: "a", Stop: StopCancel})
	if len(b.Tasks()) != 1 {
		t.Fatal("Tasks")
	}
	if _, ok := b.Get("a"); !ok {
		t.Fatal("Get")
	}
	if err := b.Stop("a"); err != nil || task.Context().Err() == nil {
		t.Fatalf("Stop: %v", err)
	}
	task.Cancelled()
	if err := b.Retry("a"); !errors.IsCode(err, ErrUnsupported) {
		t.Errorf("Retry: %v", err)
	}
	if err := b.Pause("a"); err == nil {
		t.Error("Pause on a finished task")
	}
	if err := b.Resume("a"); err == nil {
		t.Error("Resume on a finished task")
	}
	if err := b.Dismiss("a"); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	if b.DismissFinished() != 0 || len(b.Tasks()) != 0 {
		t.Error("DismissFinished")
	}
}
