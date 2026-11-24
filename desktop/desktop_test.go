package desktop

import (
	stderrors "errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/events"
	"github.com/jrschumacher/wails-kit/tasks"
)

// ---- Fakes ----

type fakeNotifier struct {
	mu         sync.Mutex
	authorized bool
	grant      bool // what RequestAuthorization answers
	requests   int
	sent       []Notification
}

func (n *fakeNotifier) Authorized() (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.authorized, nil
}

func (n *fakeNotifier) RequestAuthorization() (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.requests++
	n.authorized = n.grant
	return n.grant, nil
}

func (n *fakeNotifier) Notify(x Notification) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, x)
	return nil
}

func (n *fakeNotifier) bodies() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []string
	for _, x := range n.sent {
		out = append(out, x.Body)
	}
	return out
}

// fakeOS records badge and taskbar calls in order.
type fakeOS struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeOS) add(s string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
	return nil
}
func (f *fakeOS) SetBadge(label string) error           { return f.add("badge " + label) }
func (f *fakeOS) RemoveBadge() error                    { return f.add("badge -") }
func (f *fakeOS) SetProgressState(s TaskbarState) error { return f.add("state " + s.String()) }
func (f *fakeOS) SetProgressValue(c, t uint64) error {
	return f.add(fmt.Sprintf("value %d/%d", c, t))
}

func (f *fakeOS) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.calls
	f.calls = nil
	return out
}

// ---- Harness ----

type harness struct {
	t      *testing.T
	clock  *fakeClock
	mem    *events.MemoryEmitter
	em     *events.Emitter
	tasks  *tasks.Service
	desk   *Service
	notify *fakeNotifier
	os     *fakeOS
}

func newHarness(t *testing.T, opts ...ServiceOption) *harness {
	t.Helper()
	h := &harness{t: t, clock: newFakeClock(), mem: events.NewMemoryEmitter(), notify: &fakeNotifier{authorized: true}, os: &fakeOS{}}
	h.em = events.NewEmitter(h.mem)
	h.tasks = tasks.NewService(
		tasks.WithEmitter(h.em), tasks.WithClock(h.clock), tasks.WithProgressInterval(0),
		tasks.WithTimeout(tasks.NoTimeout), tasks.WithWaitingAfter(tasks.NoTimeout), tasks.WithWakeDetection(0, 0),
	)
	base := []ServiceOption{
		WithEmitter(h.em), WithTasks(h.tasks), WithClock(h.clock), WithAppName("Example"),
		WithNotifier(h.notify), WithBadge(h.os), WithTaskbar(h.os),
	}
	h.desk = NewService(append(base, opts...)...)
	h.desk.Start()
	t.Cleanup(func() { h.desk.Close(); h.tasks.Close() })
	return h
}

func (h *harness) add(spec tasks.Spec) *tasks.Task {
	h.t.Helper()
	task, err := h.tasks.Add(spec)
	if err != nil {
		h.t.Fatalf("Add: %v", err)
	}
	return task
}

func (h *harness) broadcasts(name string) []any {
	var out []any
	for _, r := range h.mem.Broadcasts() {
		if r.Name == name {
			out = append(out, r.Data)
		}
	}
	return out
}

// ---- Notifications ----

func TestNotifiesLongWorkThatEndsUnfocused(t *testing.T) {
	cases := []struct {
		name    string
		ran     time.Duration
		focused bool
		end     func(*tasks.Task)
		want    []string
	}{
		{"done", 11 * time.Second, false, func(k *tasks.Task) { k.Done("Report exported") }, []string{"Report exported."}},
		{"failed", 11 * time.Second, false, func(k *tasks.Task) { k.Fail(stderrors.New("x"), "Couldn't export the report · Retry") }, []string{"Couldn't export the report."}},
		{"short work", 10 * time.Second, false, func(k *tasks.Task) { k.Done("Report exported") }, nil},
		{"window focused", time.Minute, true, func(k *tasks.Task) { k.Done("Report exported") }, nil},
		{"stopped by the person", time.Minute, false, func(k *tasks.Task) { k.Stopped("12 of 40 files copied") }, nil},
		{"cancelled", time.Minute, false, func(k *tasks.Task) { k.Cancelled() }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.desk.SetFocused(tc.focused)
			k := h.add(tasks.Spec{Kind: "export", Subject: "secret-report.pdf", Words: "Exporting"})
			h.clock.Advance(tc.ran)
			tc.end(k)
			if got := h.notify.bodies(); got != nil {
				t.Fatalf("sent before the group window closed: %q", got)
			}
			h.clock.Advance(DefaultGroupWindow)
			if got := h.notify.bodies(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("notifications = %q, want %q", got, tc.want)
			}
			for _, n := range h.notify.sent {
				if n.Title != "Example" {
					t.Errorf("title = %q, want the app's name", n.Title)
				}
				if regexp.MustCompile(`secret|x$`).MatchString(n.Body) {
					t.Errorf("body %q carries the subject or the raw error", n.Body)
				}
			}
		})
	}
}

func TestPartsAreNotNotifiedAlone(t *testing.T) {
	h := newHarness(t)
	h.desk.SetFocused(false)
	k := h.add(tasks.Spec{Words: "Running reviewers"})
	part, _ := k.AddPart(tasks.Spec{Subject: "lint"})
	h.clock.Advance(time.Minute)
	part.Done("Lint finished")
	h.clock.Advance(DefaultGroupWindow)
	if got := h.notify.bodies(); got != nil {
		t.Fatalf("a part was notified: %q", got)
	}
	k.Done("Reviewers finished")
	h.clock.Advance(DefaultGroupWindow)
	if got := h.notify.bodies(); !reflect.DeepEqual(got, []string{"Reviewers finished."}) {
		t.Fatalf("notifications = %q", got)
	}
}

func TestNotificationsAreGroupedAndClickShowsTheTasks(t *testing.T) {
	shown := 0
	h := newHarness(t, WithShow(func() { shown++ }))
	h.desk.SetFocused(false)
	a := h.add(tasks.Spec{ID: "a", Words: "Exporting"})
	b := h.add(tasks.Spec{ID: "b", Words: "Syncing"})
	c := h.add(tasks.Spec{ID: "c", Words: "Uploading"})
	h.clock.Advance(time.Minute)
	a.Done("Exported")
	h.clock.Advance(300 * time.Millisecond)
	b.Fail(stderrors.New("boom"), "Couldn't sync")
	h.clock.Advance(300 * time.Millisecond)
	c.Done("Uploaded")
	h.clock.Advance(DefaultGroupWindow)
	if got := h.notify.bodies(); !reflect.DeepEqual(got, []string{"2 tasks finished and 1 failed."}) {
		t.Fatalf("notifications = %q, want one grouped", got)
	}

	if h.desk.NotificationClicked("someone-elses") {
		t.Fatal("claimed a notification it didn't send")
	}
	if !h.desk.NotificationClicked(h.notify.sent[0].ID) {
		t.Fatal("didn't recognise its own notification")
	}
	if shown != 1 {
		t.Fatalf("show called %d times, want 1", shown)
	}
	got := h.broadcasts(EventShowTasks)
	want := []any{ShowTasksPayload{Tasks: []string{"a", "b", "c"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", EventShowTasks, got, want)
	}
}

func TestFocusDropsPendingNotifications(t *testing.T) {
	h := newHarness(t)
	h.desk.SetFocused(false)
	k := h.add(tasks.Spec{Words: "Exporting"})
	h.clock.Advance(time.Minute)
	k.Done("Exported")
	h.desk.SetFocused(true)
	h.clock.Advance(DefaultGroupWindow)
	if got := h.notify.bodies(); got != nil {
		t.Fatalf("notified work the person is watching: %q", got)
	}
}

func TestWorkThatEndsWhileWatchedIsNotNotifiedLater(t *testing.T) {
	h := newHarness(t)
	k := h.add(tasks.Spec{Words: "Exporting"})
	h.clock.Advance(time.Minute)
	k.Done("Exported") // the person is watching
	h.desk.SetFocused(false)
	h.clock.Advance(DefaultGroupWindow)
	if got := h.notify.bodies(); got != nil {
		t.Fatalf("notified work that ended while focused: %q", got)
	}
}

func TestPermissionIsAskedOnFirstUseOnce(t *testing.T) {
	h := newHarness(t)
	h.notify.authorized, h.notify.grant = false, false
	if h.notify.requests != 0 {
		t.Fatal("asked for permission at launch")
	}
	h.desk.SetFocused(false)
	finish := func() {
		k := h.add(tasks.Spec{Words: "Exporting"})
		h.clock.Advance(time.Minute)
		k.Done("Exported")
		h.clock.Advance(DefaultGroupWindow)
	}
	finish()
	if h.notify.requests != 1 || len(h.notify.sent) != 0 {
		t.Fatalf("requests = %d, sent = %d; want one request and nothing sent", h.notify.requests, len(h.notify.sent))
	}
	finish()
	if h.notify.requests != 1 {
		t.Fatalf("asked again after a refusal: %d requests", h.notify.requests)
	}
	// The person turns notifications on in the system's settings.
	h.notify.mu.Lock()
	h.notify.authorized = true
	h.notify.mu.Unlock()
	finish()
	if len(h.notify.sent) != 1 {
		t.Fatalf("sent = %d after permission was given, want 1", len(h.notify.sent))
	}
}

func TestSeedsFromTheTaskList(t *testing.T) {
	// Work already running when the service starts still counts.
	clock := newFakeClock()
	em := events.NewEmitter(events.NewMemoryEmitter())
	ts := tasks.NewService(tasks.WithEmitter(em), tasks.WithClock(clock), tasks.WithTimeout(tasks.NoTimeout),
		tasks.WithWaitingAfter(tasks.NoTimeout), tasks.WithWakeDetection(0, 0))
	defer ts.Close()
	k, _ := ts.Add(tasks.Spec{Words: "Exporting"})
	n := &fakeNotifier{authorized: true}
	d := NewService(WithEmitter(em), WithTasks(ts), WithClock(clock), WithNotifier(n))
	defer d.Close()
	if d.Running() != 1 {
		t.Fatalf("Running = %d, want 1", d.Running())
	}
	d.SetFocused(false)
	clock.Advance(time.Minute)
	k.Done("Exported")
	clock.Advance(DefaultGroupWindow)
	if len(n.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(n.sent))
	}
	if n.sent[0].Title == "" {
		t.Fatal("empty title: Wails refuses a notification without one")
	}
}

func TestDefaultNotificationWords(t *testing.T) {
	s := func(state tasks.State, words string) tasks.Snapshot {
		return tasks.Snapshot{State: state, Words: words}
	}
	cases := []struct {
		in   []tasks.Snapshot
		want string
	}{
		{[]tasks.Snapshot{s(tasks.StateDone, "Export finished")}, "Export finished."},
		{[]tasks.Snapshot{s(tasks.StateDone, "Is it done?")}, "Is it done?"},
		{[]tasks.Snapshot{s(tasks.StateFailed, "Export failed while uploading · Retry")}, "Export failed while uploading."},
		{[]tasks.Snapshot{s(tasks.StateDone, "")}, "A task finished."},
		{[]tasks.Snapshot{s(tasks.StateFailed, " ")}, "A task failed."},
		{[]tasks.Snapshot{s(tasks.StateDone, "a"), s(tasks.StateDone, "b"), s(tasks.StateDone, "c")}, "3 tasks finished."},
		{[]tasks.Snapshot{s(tasks.StateFailed, "a"), s(tasks.StateFailed, "b")}, "2 tasks failed."},
		{[]tasks.Snapshot{s(tasks.StateDone, "a"), s(tasks.StateFailed, "b")}, "1 task finished and 1 failed."},
	}
	for _, tc := range cases {
		if got := DefaultWords().Notification(tc.in); got != tc.want {
			t.Errorf("Notification(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---- Badge ----

func TestBadgeCountsWhatNeedsThePerson(t *testing.T) {
	h := newHarness(t, WithTaskbar(nil))
	a := h.add(tasks.Spec{ID: "a"})
	b := h.add(tasks.Spec{ID: "b"})
	c := h.add(tasks.Spec{ID: "c", Retry: tasks.RetryAuto, OnRetry: func(*tasks.Task) {}})
	p := h.add(tasks.Spec{ID: "p"})
	p1, _ := p.AddPart(tasks.Spec{})
	p2, _ := p.AddPart(tasks.Spec{})
	if got := h.os.take(); got != nil {
		t.Fatalf("badge set with nothing to count: %q", got)
	}
	a.Fail(stderrors.New("x"), "Couldn't save")
	c.Fail(stderrors.New("x"), "Couldn't sync") // retries by itself: neutral
	b.Stalled("Sign-in expired · Sign in")
	p1.Fail(stderrors.New("x"), "Couldn't lint")
	p2.Fail(stderrors.New("x"), "Couldn't test") // the same task: counts once
	if err := h.tasks.Dismiss("a"); err != nil {
		t.Fatal(err)
	}
	b.Running("")
	p.Fail(stderrors.New("x"), "2 parts failed")
	if err := h.tasks.Dismiss("p"); err != nil {
		t.Fatal(err)
	}
	want := []string{"badge 1", "badge 2", "badge 3", "badge 2", "badge 1", "badge -"}
	if got := h.os.take(); !reflect.DeepEqual(got, want) {
		t.Fatalf("badge calls = %q, want %q", got, want)
	}
}

// ---- Taskbar ----

func TestTaskbarProgress(t *testing.T) {
	h := newHarness(t, WithBadge(nil))
	a := h.add(tasks.Spec{ID: "a"})
	a.Progress(tasks.Progress{Done: 1, Total: 4})
	b := h.add(tasks.Spec{ID: "b"})
	b.Progress(tasks.Progress{Done: 5, Total: 6})
	a.Progress(tasks.Progress{Done: 3, Total: 4})
	a.Paused("Paused · 3 of 4 files")
	b.Paused("Paused")
	b.Running("")
	b.Done("Done")
	want := []string{
		"state indeterminate",       // a has no total yet
		"state normal", "value 1/4", // a counts
		"state indeterminate",        // b has no total yet
		"state normal", "value 6/10", // combined
		"value 8/10",                //
		"value 5/6",                 // a paused: not drawn as Paused, just left out
		"state none",                // nothing running
		"state normal", "value 5/6", //
		"state none", // finished
	}
	if got := h.os.take(); !reflect.DeepEqual(got, want) {
		t.Fatalf("taskbar calls =\n%q\nwant\n%q", got, want)
	}
}

func TestTaskbarErrorForAttentionFailureUnfocused(t *testing.T) {
	h := newHarness(t, WithBadge(nil))
	h.desk.SetFocused(false)
	a := h.add(tasks.Spec{ID: "a"})
	h.add(tasks.Spec{ID: "b"})
	h.os.take()
	a.Fail(stderrors.New("x"), "Couldn't save")
	if got, want := h.os.take(), []string{"value 1/1", "state error"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("on failure: %q, want %q (full first, then Error)", got, want)
	}
	h.desk.SetFocused(true)
	if got, want := h.os.take(), []string{"state indeterminate"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("on focus: %q, want %q", got, want)
	}

	// Focused: a failure shows in the window, not on the taskbar.
	c := h.add(tasks.Spec{ID: "c"})
	c.Fail(stderrors.New("x"), "Couldn't save")
	for _, call := range h.os.take() {
		if call == "state error" {
			t.Fatal("Error set while the window was focused")
		}
	}

	// A failure that retries by itself is neutral.
	h.desk.SetFocused(false)
	d := h.add(tasks.Spec{ID: "d", Retry: tasks.RetryAuto, OnRetry: func(*tasks.Task) {}})
	d.Fail(stderrors.New("x"), "Couldn't sync")
	for _, call := range h.os.take() {
		if call == "state error" {
			t.Fatal("Error set for an automatic retry")
		}
	}
}

func TestTaskbarErrorClearsWhenNothingNeedsThePerson(t *testing.T) {
	h := newHarness(t, WithBadge(nil))
	h.desk.SetFocused(false)
	a := h.add(tasks.Spec{ID: "a"})
	a.Fail(stderrors.New("x"), "Couldn't save")
	h.os.take()
	if err := h.tasks.Dismiss("a"); err != nil {
		t.Fatal(err)
	}
	if got, want := h.os.take(), []string{"state none"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after dismiss: %q, want %q", got, want)
	}
}

// ---- Quit ----

type askRecorder struct {
	asked  chan QuitQuestion
	answer chan QuitChoice
}

func newAsker() *askRecorder {
	return &askRecorder{asked: make(chan QuitQuestion, 4), answer: make(chan QuitChoice)}
}

func (a *askRecorder) AskQuit(q QuitQuestion) QuitChoice {
	a.asked <- q
	return <-a.answer
}

func waitQuit(t *testing.T, quit chan struct{}, want bool) {
	t.Helper()
	select {
	case <-quit:
		if !want {
			t.Fatal("quit, want not")
		}
	case <-time.After(200 * time.Millisecond):
		if want {
			t.Fatal("didn't quit")
		}
	}
}

func TestQuitWithNothingRunning(t *testing.T) {
	asker := newAsker()
	h := newHarness(t, WithAsker(asker))
	k := h.add(tasks.Spec{})
	k.Done("Done")
	if !h.desk.ShouldQuit() {
		t.Fatal("held up a quit with nothing running")
	}
	select {
	case <-asker.asked:
		t.Fatal("asked with nothing running")
	default:
	}
}

func TestQuitAsksAndQuitsNow(t *testing.T) {
	asker := newAsker()
	quit := make(chan struct{}, 2)
	h := newHarness(t, WithAsker(asker), WithQuit(func() { quit <- struct{}{} }))
	h.add(tasks.Spec{})
	h.add(tasks.Spec{})
	for range 2 { // a second quit while the question is open doesn't ask again
		if h.desk.ShouldQuit() {
			t.Fatal("quit while work runs")
		}
	}
	q := <-asker.asked
	want := QuitQuestion{Message: "Quit while 2 tasks are running?", Detail: DefaultWords().QuitDetail, Running: 2,
		KeepRunning: "Keep Running", QuitWhenDone: "Quit When Done", QuitNow: "Quit"}
	if q != want {
		t.Fatalf("question = %#v, want %#v", q, want)
	}
	select {
	case <-asker.asked:
		t.Fatal("asked twice at once")
	case <-time.After(50 * time.Millisecond):
	}
	asker.answer <- QuitNow
	waitQuit(t, quit, true)
	if !h.desk.ShouldQuit() {
		t.Fatal("ShouldQuit false after the person chose Quit")
	}
}

func TestQuitKeepRunning(t *testing.T) {
	asker := newAsker()
	quit := make(chan struct{}, 2)
	h := newHarness(t, WithAsker(asker), WithQuit(func() { quit <- struct{}{} }))
	k := h.add(tasks.Spec{})
	h.desk.ShouldQuit()
	<-asker.asked
	asker.answer <- QuitKeepRunning
	waitQuit(t, quit, false)
	k.Done("Done")
	waitQuit(t, quit, false)
	if h.desk.QuitPending() {
		t.Fatal("quit pending after Keep Running")
	}
}

func TestQuitWhenDone(t *testing.T) {
	asker := newAsker()
	quit := make(chan struct{}, 2)
	h := newHarness(t, WithAsker(asker), WithQuit(func() { quit <- struct{}{} }))
	a := h.add(tasks.Spec{})
	b := h.add(tasks.Spec{})
	h.desk.ShouldQuit()
	<-asker.asked
	asker.answer <- QuitWhenDone
	deadline := time.Now().Add(time.Second)
	for !h.desk.Bindings().QuitPending() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !h.desk.QuitPending() {
		t.Fatal("not pending after Quit When Done")
	}
	a.Done("Done")
	waitQuit(t, quit, false)
	b.Fail(stderrors.New("x"), "Couldn't")
	waitQuit(t, quit, true)
	if !h.desk.ShouldQuit() {
		t.Fatal("ShouldQuit false once the work finished")
	}
	got := h.broadcasts(EventQuitPending)
	want := []any{QuitPendingPayload{Pending: true, Running: 2}, QuitPendingPayload{Pending: false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", EventQuitPending, got, want)
	}
}

func TestCancelQuitWhenDone(t *testing.T) {
	asker := newAsker()
	quit := make(chan struct{}, 2)
	h := newHarness(t, WithAsker(asker), WithQuit(func() { quit <- struct{}{} }))
	k := h.add(tasks.Spec{})
	h.desk.ShouldQuit()
	<-asker.asked
	asker.answer <- QuitWhenDone
	for !h.desk.QuitPending() {
		time.Sleep(time.Millisecond)
	}
	h.desk.Bindings().CancelQuit()
	k.Done("Done")
	waitQuit(t, quit, false)
}

func TestQuitWithoutAskerIsNeverHeldUp(t *testing.T) {
	h := newHarness(t)
	h.add(tasks.Spec{})
	if !h.desk.ShouldQuit() {
		t.Fatal("held up a quit with no way to ask")
	}
}

func TestQuitWords(t *testing.T) {
	w := DefaultWords()
	if got := w.QuitMessage(1); got != "Quit while 1 task is running?" {
		t.Errorf("QuitMessage(1) = %q", got)
	}
	asker := newAsker()
	h := newHarness(t, WithAsker(asker), WithWords(Words{
		QuitMessage: func(n int) string { return fmt.Sprintf("Quit while %d jobs are running?", n) },
		QuitNow:     "Quit Anyway",
	}))
	h.add(tasks.Spec{})
	h.desk.ShouldQuit()
	q := <-asker.asked
	asker.answer <- QuitKeepRunning
	if q.Message != "Quit while 1 jobs are running?" || q.QuitNow != "Quit Anyway" || q.KeepRunning != "Keep Running" {
		t.Fatalf("question = %#v", q)
	}
}

// ---- Background ----

func TestBackgroundFollowsAppearance(t *testing.T) {
	light, dark := MustParseColour("#ffffff"), MustParseColour("#000000")
	d := NewService(WithBackground(light, dark))
	repaints := 0
	d.onAppearance = func() { repaints++ }
	cases := []struct {
		a          Appearance
		systemDark bool
		want       Colour
	}{
		{AppearanceSystem, false, light},
		{AppearanceSystem, true, dark},
		{AppearanceLight, true, light},
		{AppearanceDark, false, dark},
	}
	for _, tc := range cases {
		d.SetAppearance(tc.a)
		if got := d.Background(tc.systemDark); got != tc.want {
			t.Errorf("Background(%q, systemDark=%v) = %s, want %s", tc.a, tc.systemDark, got.Hex(), tc.want.Hex())
		}
	}
	if repaints != len(cases) {
		t.Errorf("repaints = %d, want %d", repaints, len(cases))
	}
}

func TestDefaultBackgroundMatchesTokens(t *testing.T) {
	css, err := os.ReadFile("../frontend/ui/src/tokens.css")
	if err != nil {
		t.Skip("tokens.css not here (a split module):", err)
	}
	bg := regexp.MustCompile(`--wk-color-bg:\s*(#[0-9a-f]{6})`).FindAllStringSubmatch(string(css), -1)
	if len(bg) != 3 {
		t.Fatalf("found %d --wk-color-bg values, want 3 (light and two dark blocks)", len(bg))
	}
	if got := DefaultBackgroundLight.Hex(); got != bg[0][1] {
		t.Errorf("DefaultBackgroundLight = %s, tokens.css has %s", got, bg[0][1])
	}
	for _, m := range bg[1:] {
		if got := DefaultBackgroundDark.Hex(); got != m[1] {
			t.Errorf("DefaultBackgroundDark = %s, tokens.css has %s", got, m[1])
		}
	}
}

func TestParseColour(t *testing.T) {
	if c, err := ParseColour("#181a1f"); err != nil || c != (Colour{0x18, 0x1a, 0x1f}) {
		t.Fatalf("ParseColour = %v, %v", c, err)
	}
	for _, bad := range []string{"", "181a1f", "#181a1", "#18la1f", "rgb(0,0,0)"} {
		if _, err := ParseColour(bad); err == nil {
			t.Errorf("ParseColour(%q) accepted", bad)
		}
	}
}

// ---- Errors and closing ----

type failingBadge struct{}

func (failingBadge) SetBadge(string) error { return stderrors.New("no dock") }
func (failingBadge) RemoveBadge() error    { return nil }

func TestOSErrorsGoToTheHandler(t *testing.T) {
	var got []error
	h := newHarness(t, WithBadge(failingBadge{}), WithErrorHandler(func(err error) { got = append(got, err) }))
	k := h.add(tasks.Spec{})
	k.Fail(stderrors.New("x"), "Couldn't")
	if len(got) != 1 {
		t.Fatalf("errors = %v, want one", got)
	}
}

func TestCloseStopsFollowing(t *testing.T) {
	h := newHarness(t)
	h.desk.SetFocused(false)
	k := h.add(tasks.Spec{})
	h.clock.Advance(time.Minute)
	k.Done("Done")
	h.desk.Close()
	h.clock.Advance(DefaultGroupWindow)
	h.add(tasks.Spec{}).Fail(stderrors.New("x"), "Couldn't")
	if len(h.notify.sent) != 0 {
		t.Fatal("sent after Close")
	}
	for _, c := range h.os.take() {
		if c == "badge 1" {
			t.Fatal("badge set after Close")
		}
	}
}

// ---- Review fixes ----

func TestNothingTouchesTheOSBeforeStart(t *testing.T) {
	clock := newFakeClock()
	em := events.NewEmitter(events.NewMemoryEmitter())
	ts := tasks.NewService(tasks.WithEmitter(em), tasks.WithClock(clock), tasks.WithTimeout(tasks.NoTimeout),
		tasks.WithWaitingAfter(tasks.NoTimeout), tasks.WithWakeDetection(0, 0), tasks.WithProgressInterval(0))
	defer ts.Close()
	failed, _ := ts.Add(tasks.Spec{ID: "early"})
	failed.Fail(stderrors.New("x"), "Couldn't") // before the service exists: read by WithTasks
	osFake := &fakeOS{}
	d := NewService(WithEmitter(em), WithTasks(ts), WithClock(clock), WithBadge(osFake), WithTaskbar(osFake))
	defer d.Close()
	d.SetFocused(false)
	k, _ := ts.Add(tasks.Spec{ID: "late"})
	k.Progress(tasks.Progress{Done: 1, Total: 2})
	d.RefreshTaskbar()
	if got := osFake.take(); got != nil {
		t.Fatalf("touched the OS before Start: %q", got)
	}
	d.Start()
	want := []string{"badge 1", "state normal", "value 1/2"}
	if got := osFake.take(); !reflect.DeepEqual(got, want) {
		t.Fatalf("on Start: %q, want %q", got, want)
	}
}

func TestSystemDarkBeforeRun(t *testing.T) {
	running := func(dark bool) func() bool { return func() bool { return dark } }
	before := func(dark, known bool) func() (bool, bool) { return func() (bool, bool) { return dark, known } }
	d := NewService()
	if !d.systemDark(running(false), before(true, true)) {
		t.Fatal("before Run: the registry's dark is ignored for Wails's light")
	}
	if d.systemDark(running(false), before(true, false)) {
		t.Fatal("before Run with nothing known: want the running answer")
	}
	d.Start()
	if d.systemDark(running(false), before(true, true)) {
		t.Fatal("after Start: the running app's answer wins")
	}
	if got := d.Background(d.systemDark(running(true), nil)); got != DefaultBackgroundDark {
		t.Fatalf("Background = %s, want dark", got.Hex())
	}
}

// flakyTaskbar fails its first n calls.
type flakyTaskbar struct {
	fakeOS
	fail int
}

func (f *flakyTaskbar) SetProgressState(s TaskbarState) error {
	if f.fail > 0 {
		f.fail--
		return stderrors.New("no taskbar button yet")
	}
	return f.fakeOS.SetProgressState(s)
}

// brokenTaskbar always fails the same way.
type brokenTaskbar struct{ calls int }

func (b *brokenTaskbar) SetProgressState(TaskbarState) error {
	b.calls++
	return stderrors.New("no taskbar button")
}
func (b *brokenTaskbar) SetProgressValue(uint64, uint64) error {
	b.calls++
	return stderrors.New("no taskbar button")
}

func TestRepeatedTaskbarErrorsAreReportedOnce(t *testing.T) {
	bar := &brokenTaskbar{}
	var errs []error
	h := newHarness(t, WithBadge(nil), WithTaskbar(bar), WithErrorHandler(func(err error) { errs = append(errs, err) }))
	k := h.add(tasks.Spec{})
	for i := range 5 {
		k.Progress(tasks.Progress{Done: int64(i), Total: 10})
	}
	if bar.calls < 5 {
		t.Fatalf("taskbar calls = %d: it should keep trying", bar.calls)
	}
	if len(errs) != 1 {
		t.Fatalf("errors reported = %d, want 1 for the same failure", len(errs))
	}
}

// flakyBadge fails its first SetBadge.
type flakyBadge struct {
	fakeOS
	failed bool
}

func (f *flakyBadge) SetBadge(label string) error {
	if !f.failed {
		f.failed = true
		return stderrors.New("dock not ready")
	}
	return f.fakeOS.SetBadge(label)
}

func TestBadgeIsKeptOnlyWhenDrawn(t *testing.T) {
	badge := &flakyBadge{}
	h := newHarness(t, WithTaskbar(nil), WithBadge(badge))
	k := h.add(tasks.Spec{})
	k.Fail(stderrors.New("x"), "Couldn't") // the first draw fails
	h.add(tasks.Spec{})                    // the same count: drawn now
	if got, want := badge.take(), []string{"badge 1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("badge calls = %q, want %q", got, want)
	}
}

func TestTaskbarStateIsKeptOnlyWhenDrawn(t *testing.T) {
	bar := &flakyTaskbar{fail: 1}
	var errs []error
	h := newHarness(t, WithBadge(nil), WithTaskbar(bar), WithErrorHandler(func(err error) { errs = append(errs, err) }))
	k := h.add(tasks.Spec{})
	if len(errs) != 1 || bar.take() != nil {
		t.Fatalf("errs = %v; want the first draw to fail", errs)
	}
	// The same state again: drawn now, not skipped as already shown.
	k.SetWords("still exporting")
	if got, want := bar.take(), []string{"state indeterminate"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after a failed draw: %q, want %q", got, want)
	}
	// A new window's taskbar button gets the state too.
	h.desk.RefreshTaskbar()
	if got, want := bar.take(), []string{"state indeterminate"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("on refresh: %q, want %q", got, want)
	}
}

func TestWindowFocusOutOfOrder(t *testing.T) {
	h := newHarness(t, WithBadge(nil))
	h.desk.WindowFocused(1, true)
	// Focus moves from window 1 to window 2; the events arrive reversed.
	h.desk.WindowFocused(2, true)
	h.desk.WindowFocused(1, false)
	k := h.add(tasks.Spec{})
	h.os.take()
	k.Fail(stderrors.New("x"), "Couldn't")
	for _, c := range h.os.take() {
		if c == "state error" {
			t.Fatal("unfocused while window 2 had focus")
		}
	}
	h.desk.WindowFocused(2, false)
	if h.desk.focusedNow() {
		t.Fatal("focused with no window focused")
	}
	// Many windows at once, from many goroutines: the answer is the set.
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(id uint) {
			defer wg.Done()
			h.desk.WindowFocused(id, true)
			h.desk.WindowFocused(id, id%2 == 0)
		}(uint(i + 10))
	}
	wg.Wait()
	if !h.desk.focusedNow() {
		t.Fatal("unfocused with even windows focused")
	}
}

// seqAsker gives each question its own answer channel, so a test can
// answer a particular one.
type seqAsker struct{ asked chan chan QuitChoice }

func newSeqAsker() *seqAsker { return &seqAsker{asked: make(chan chan QuitChoice, 4)} }

func (a *seqAsker) AskQuit(QuitQuestion) QuitChoice {
	reply := make(chan QuitChoice)
	a.asked <- reply
	return <-reply
}

func (a *seqAsker) next(t *testing.T, when string) chan QuitChoice {
	t.Helper()
	select {
	case r := <-a.asked:
		return r
	case <-time.After(time.Second):
		t.Fatalf("didn't ask %s", when)
		return nil
	}
}

func (a *seqAsker) none(t *testing.T, when string) {
	t.Helper()
	select {
	case <-a.asked:
		t.Fatalf("asked %s", when)
	case <-time.After(50 * time.Millisecond):
	}
}

// answered waits until the latest question has its answer applied.
func answered(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		open := s.askOpen
		s.mu.Unlock()
		if !open {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the answer wasn't applied")
}

func TestReaskAfterAnUnansweredQuestion(t *testing.T) {
	asker := newSeqAsker()
	quit := make(chan struct{}, 2)
	h := newHarness(t, WithAsker(asker), WithQuit(func() { quit <- struct{}{} }), WithReaskAfter(time.Minute))
	h.add(tasks.Spec{})
	h.desk.ShouldQuit()
	first := asker.next(t, "on the first quit") // left unanswered
	h.clock.Advance(30 * time.Second)
	h.desk.ShouldQuit()
	asker.none(t, "again before the re-ask time")
	h.clock.Advance(30 * time.Second)
	h.desk.ShouldQuit()
	second := asker.next(t, "after the re-ask time")
	// The first question is abandoned: an answer from it doesn't count.
	first <- QuitNow
	waitQuit(t, quit, false)
	second <- QuitNow
	waitQuit(t, quit, true)
}

// The reviewer's case: an Esc-dismissed question must not swallow later
// quits once a newer question has been answered.
func TestQuitAfterAnAbandonedQuestionAsksAgain(t *testing.T) {
	asker := newSeqAsker()
	h := newHarness(t, WithAsker(asker), WithReaskAfter(time.Minute)) // wider than the 5 s: the generation matters, not the window
	h.add(tasks.Spec{})
	h.desk.ShouldQuit()
	asker.next(t, "on the first quit") // dismissed with Esc
	h.clock.Advance(60 * time.Second)
	h.desk.ShouldQuit()
	asker.next(t, "after an abandoned question") <- QuitKeepRunning
	answered(t, h.desk)
	h.clock.Advance(5 * time.Second)
	h.desk.ShouldQuit()
	asker.next(t, "5 s after Keep Running") <- QuitKeepRunning
}

func TestNeedsPersonDecidesTheTaskbarError(t *testing.T) {
	// Only a stalled task needs the person here; a failure is the app's
	// to handle.
	h := newHarness(t, WithNeedsPerson(func(s tasks.Snapshot) bool { return s.State == tasks.StateStalled }))
	h.desk.SetFocused(false)
	h.add(tasks.Spec{}).Stalled("Sign-in expired")
	k := h.add(tasks.Spec{})
	h.os.take()
	k.Fail(stderrors.New("x"), "Couldn't")
	for _, c := range h.os.take() {
		if c == "state error" || c == "badge 2" {
			t.Fatalf("%q for a failure the app says needs no one", c)
		}
	}
}

func TestOnlyAnEndFromUnfinishedIsNotified(t *testing.T) {
	h := newHarness(t)
	h.desk.SetFocused(false)
	start := int64(0)
	snap := func(state tasks.State) tasks.ChangedPayload {
		return tasks.ChangedPayload{Tasks: []tasks.Snapshot{{ID: "x", State: state, Words: "Exported", StartedAt: &start, UpdatedAt: 60_000}}}
	}
	h.desk.onChanged(snap(tasks.StateFailed))
	h.desk.onChanged(snap(tasks.StateFailed)) // known now
	h.desk.onChanged(snap(tasks.StateDone))   // finished to finished: not an end
	h.clock.Advance(DefaultGroupWindow)
	if got := h.notify.bodies(); got != nil {
		t.Fatalf("notified a change between finished states: %q", got)
	}
}

func TestNotifierRemovedBeforeTheGroupIsSent(t *testing.T) {
	h := newHarness(t)
	h.desk.SetFocused(false)
	k := h.add(tasks.Spec{})
	h.clock.Advance(time.Minute)
	k.Done("Done")
	h.desk.SetNotifier(nil)
	h.clock.Advance(DefaultGroupWindow) // must not panic
}

func (s *Service) focusedNow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.focused
}
