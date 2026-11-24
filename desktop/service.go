package desktop

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jrschumacher/wails-kit/events"
	"github.com/jrschumacher/wails-kit/tasks"
)

// ServiceOption configures a Service.
type ServiceOption func(*Service)

// WithEmitter sets the emitter the tasks service sends tasks:changed on.
// The service follows it there, and sends its own events on it. Without
// it, the service sees no tasks.
func WithEmitter(e *events.Emitter) ServiceOption {
	return func(s *Service) { s.emitter = e }
}

// WithTasks gives the service the tasks service: it reads the current list
// at once (so work already running counts), and Attach routes Windows's
// sleep and wake events to its Sleep and Wake.
func WithTasks(t *tasks.Service) ServiceOption {
	return func(s *Service) { s.tasks = t }
}

// WithNotifier sets where notifications go. Without it, none are sent.
func WithNotifier(n Notifier) ServiceOption {
	return func(s *Service) { s.notifier = n }
}

// WithBadge sets the app icon's badge, usually Wails's dock service
// (dock.New()). Without it, no badge is drawn.
func WithBadge(b Badge) ServiceOption {
	return func(s *Service) { s.badge = b }
}

// WithTaskbar sets the taskbar button's progress. Attach sets one on
// Windows unless this is given.
func WithTaskbar(t Taskbar) ServiceOption {
	return func(s *Service) { s.taskbar = t }
}

// WithAsker sets what asks the quit question. Attach sets a Wails question
// dialog unless this is given. Without one, quitting is never held up.
func WithAsker(a Asker) ServiceOption {
	return func(s *Service) { s.asker = a }
}

// WithQuit sets the function that quits the app, called after the person
// chooses Quit, or when the work finishes after Quit When Done. Attach sets
// app.Quit unless this is given.
func WithQuit(f func()) ServiceOption {
	return func(s *Service) { s.quit = f }
}

// WithShow sets what brings a window forward when the person clicks a
// notification. Attach sets one that restores, shows and focuses the first
// window unless this is given.
func WithShow(f func()) ServiceOption {
	return func(s *Service) { s.show = f }
}

// WithAppName sets the notifications' title. Attach uses the app's name
// unless this is given.
func WithAppName(name string) ServiceOption {
	return func(s *Service) { s.appName = name }
}

// WithWords sets the service's words. Empty fields keep the defaults.
func WithWords(w Words) ServiceOption {
	return func(s *Service) {
		d := &s.words
		if w.Notification != nil {
			d.Notification = w.Notification
		}
		if w.QuitMessage != nil {
			d.QuitMessage = w.QuitMessage
		}
		for _, f := range []struct {
			to   *string
			from string
		}{
			{&d.QuitDetail, w.QuitDetail}, {&d.KeepRunning, w.KeepRunning},
			{&d.QuitWhenDone, w.QuitWhenDone}, {&d.QuitNow, w.QuitNow},
		} {
			if f.from != "" {
				*f.to = f.from
			}
		}
	}
}

// WithNeedsPerson sets which tasks need the person: they count on the
// badge, and a failure among them turns the taskbar button red while the
// window isn't focused. The default is a failed or stalled task. A
// top-level task counts once if it, or any of its parts, needs the person.
func WithNeedsPerson(f func(tasks.Snapshot) bool) ServiceOption {
	return func(s *Service) {
		if f != nil {
			s.needs = f
		}
	}
}

// WithLongWork sets how long work must have run for its end to be worth a
// notification (default 10 s).
func WithLongWork(d time.Duration) ServiceOption {
	return func(s *Service) {
		if d >= 0 {
			s.longWork = d
		}
	}
}

// WithGroupWindow sets how long the service waits after a task ends for
// others to end with it, so they share one notification (default 1 s).
func WithGroupWindow(d time.Duration) ServiceOption {
	return func(s *Service) {
		if d >= 0 {
			s.groupWindow = d
		}
	}
}

// WithBackground sets the window background in light and dark: the
// theme's --wk-color-bg. The default is the kit's own.
func WithBackground(light, dark Colour) ServiceOption {
	return func(s *Service) { s.bgLight, s.bgDark = light, dark }
}

// WithClock sets the time source, the tasks package's Clock. Tests inject a
// fake.
func WithClock(c tasks.Clock) ServiceOption {
	return func(s *Service) {
		if c != nil {
			s.clock = c
		}
	}
}

// WithErrorHandler receives errors from the notifier, badge and taskbar,
// which the service otherwise ignores: none of them may stop the work.
func WithErrorHandler(f func(error)) ServiceOption {
	return func(s *Service) { s.onError = f }
}

// Service follows the task list and draws it on the desktop. It is safe for
// concurrent use.
type Service struct {
	emitter  *events.Emitter
	tasks    *tasks.Service
	notifier Notifier
	badge    Badge
	taskbar  Taskbar
	asker    Asker
	quit     func()
	show     func()
	appName  string
	words    Words
	needs    func(tasks.Snapshot) bool

	longWork    time.Duration
	groupWindow time.Duration
	clock       tasks.Clock
	onError     func(error)

	bgLight, bgDark Colour

	unsubscribe func()

	mu           sync.Mutex
	closed       bool
	focused      bool
	appearance   Appearance
	onAppearance func()
	states       map[string]tasks.State // the last state seen of every task
	running      int                    // top-level tasks not finished
	attention    int                    // top-level tasks that need the person
	taskbarError bool                   // a failure needs the person and the window wasn't focused
	progress     taskbarWant
	pending      []tasks.Snapshot // finished, waiting for others to finish with them
	group        tasks.Timer
	nextID       int
	sent         map[string][]string // notification ID -> task IDs
	sentOrder    []string
	asking       bool
	quitAllowed  bool
	quitPending  bool

	// The operating system's side, applied in order under osMu.
	osMu        sync.Mutex
	shownBadge  int
	shownBar    taskbarWant
	notifyMu    sync.Mutex
	askedNotify bool
}

type taskbarWant struct {
	state       TaskbarState
	done, total uint64
}

// sentLimit is how many notifications' task IDs are kept for clicks.
const sentLimit = 32

// NewService creates the service and starts following tasks:changed.
func NewService(opts ...ServiceOption) *Service {
	s := &Service{
		words:       DefaultWords(),
		needs:       defaultNeeds,
		longWork:    DefaultLongWork,
		groupWindow: DefaultGroupWindow,
		clock:       realClock{},
		bgLight:     DefaultBackgroundLight,
		bgDark:      DefaultBackgroundDark,
		focused:     true,
		states:      map[string]tasks.State{},
		sent:        map[string][]string{},
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.emitter != nil {
		s.unsubscribe = events.On(s.emitter, tasks.EventChanged, s.onChanged)
	}
	if s.tasks != nil {
		s.onChanged(tasks.ChangedPayload{Tasks: s.tasks.Tasks()})
	}
	return s
}

func defaultNeeds(t tasks.Snapshot) bool {
	return t.State == tasks.StateFailed || t.State == tasks.StateStalled
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) AfterFunc(d time.Duration, f func()) tasks.Timer { return time.AfterFunc(d, f) }

// onChanged reads one snapshot of the task list.
func (s *Service) onChanged(p tasks.ChangedPayload) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	parts := map[string][]tasks.Snapshot{}
	for _, t := range p.Tasks {
		if t.Parent != "" {
			parts[t.Parent] = append(parts[t.Parent], t)
		}
	}
	states := make(map[string]tasks.State, len(p.Tasks))
	var finished []tasks.Snapshot
	running, attention, newFailure := 0, 0, false
	var active []tasks.Snapshot
	for _, t := range p.Tasks {
		prev, known := s.states[t.ID]
		states[t.ID] = t.State
		if t.Parent != "" {
			continue
		}
		if !t.State.Finished() {
			running++
		}
		if t.State == tasks.StateRunning || t.State == tasks.StateWaiting {
			active = append(active, t)
		}
		needs := s.needs(t)
		for _, part := range parts[t.ID] {
			needs = needs || s.needs(part)
		}
		if needs {
			attention++
		}
		if !known || prev == t.State {
			continue
		}
		ended := !prev.Finished() && (t.State == tasks.StateDone || t.State == tasks.StateFailed)
		if ended && s.long(t) {
			finished = append(finished, t)
		}
		if t.State == tasks.StateFailed && s.needs(t) {
			newFailure = true
		}
	}
	s.states = states
	s.running, s.attention = running, attention
	if newFailure && !s.focused {
		s.taskbarError = true
	}
	if attention == 0 {
		s.taskbarError = false
	}
	s.progress = progressOf(active)
	if len(finished) > 0 && !s.focused && s.notifier != nil {
		s.pending = append(s.pending, finished...)
		if s.group == nil {
			s.group = s.clock.AfterFunc(s.groupWindow, s.flushNotifications)
		}
	}
	quitNow := false
	if s.quitPending && running == 0 {
		s.quitPending, s.quitAllowed, quitNow = false, true, true
	}
	s.mu.Unlock()

	s.apply()
	if quitNow {
		s.emit(EventQuitPending, QuitPendingPayload{Pending: false})
		s.callQuit()
	}
}

// long reports whether a task that just ended ran long enough to notify.
func (s *Service) long(t tasks.Snapshot) bool {
	if t.StartedAt == nil {
		return false
	}
	return time.Duration(t.UpdatedAt-*t.StartedAt)*time.Millisecond > s.longWork
}

// progressOf is the taskbar's progress for the running top-level tasks:
// their combined count when every one has a total, else indeterminate.
func progressOf(active []tasks.Snapshot) taskbarWant {
	if len(active) == 0 {
		return taskbarWant{state: TaskbarNone}
	}
	var done, total uint64
	for _, t := range active {
		if t.Total <= 0 {
			return taskbarWant{state: TaskbarIndeterminate}
		}
		d := min(max(t.Done, 0), t.Total)
		done += uint64(d)
		total += uint64(t.Total)
	}
	return taskbarWant{state: TaskbarNormal, done: done, total: total}
}

// apply brings the badge and the taskbar into line with the latest
// snapshot, calling the operating system only for what changed.
func (s *Service) apply() {
	s.osMu.Lock()
	defer s.osMu.Unlock()
	s.mu.Lock()
	count, bar := s.attention, s.progress
	if s.taskbarError {
		bar = taskbarWant{state: TaskbarError, done: 1, total: 1}
	}
	s.mu.Unlock()

	if s.badge != nil && count != s.shownBadge {
		var err error
		if count == 0 {
			err = s.badge.RemoveBadge()
		} else {
			err = s.badge.SetBadge(strconv.Itoa(count))
		}
		s.report(err)
		s.shownBadge = count
	}
	if s.taskbar != nil && bar != s.shownBar {
		switch bar.state {
		case TaskbarError:
			// The value goes to full first: Error over Indeterminate would
			// draw a made-up percentage.
			s.report(s.taskbar.SetProgressValue(1, 1))
			s.report(s.taskbar.SetProgressState(TaskbarError))
		case TaskbarNormal:
			if s.shownBar.state != TaskbarNormal {
				s.report(s.taskbar.SetProgressState(TaskbarNormal))
			}
			s.report(s.taskbar.SetProgressValue(bar.done, bar.total))
		default:
			s.report(s.taskbar.SetProgressState(bar.state))
		}
		s.shownBar = bar
	}
}

func (s *Service) report(err error) {
	if err != nil && s.onError != nil {
		s.onError(err)
	}
}

func (s *Service) emit(name string, data any) {
	if s.emitter != nil {
		s.emitter.Emit(name, data)
	}
}

// SetFocused records whether any of the app's windows is focused. Attach
// calls it from the windows' focus events. Focus clears the taskbar's
// Error, and drops notifications not yet sent: the person is watching.
func (s *Service) SetFocused(focused bool) {
	s.mu.Lock()
	s.focused = focused
	if focused {
		s.taskbarError = false
		s.pending = nil
		if s.group != nil {
			s.group.Stop()
			s.group = nil
		}
	}
	s.mu.Unlock()
	s.apply()
}

// flushNotifications sends one notification for the tasks that finished
// together.
func (s *Service) flushNotifications() {
	s.mu.Lock()
	group := s.pending
	s.pending, s.group = nil, nil
	if s.closed || s.focused || len(group) == 0 {
		s.mu.Unlock()
		return
	}
	s.nextID++
	id := fmt.Sprintf("wk-desktop-%d", s.nextID)
	ids := make([]string, len(group))
	for i, t := range group {
		ids[i] = t.ID
	}
	s.sent[id] = ids
	s.sentOrder = append(s.sentOrder, id)
	if len(s.sentOrder) > sentLimit {
		delete(s.sent, s.sentOrder[0])
		s.sentOrder = s.sentOrder[1:]
	}
	n := Notification{ID: id, Title: s.appName, Body: s.words.Notification(group)}
	s.mu.Unlock()

	if n.Title == "" {
		n.Title = n.Body
	}
	s.send(n)
}

// send asks for permission the first time a notification would help, then
// sends it if the app may.
func (s *Service) send(n Notification) {
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()
	ok, err := s.notifier.Authorized()
	if !ok && !s.askedNotify {
		s.askedNotify = true
		ok, err = s.notifier.RequestAuthorization()
	}
	if err != nil {
		s.report(err)
		return
	}
	if !ok {
		return // without permission, the badge and the window's words carry it
	}
	s.report(s.notifier.Notify(n))
}

// SetNotifier sets the notifier after NewService, for a notifier that needs the
// service, such as UseNotifications'.
func (s *Service) SetNotifier(n Notifier) {
	s.notifyMu.Lock()
	s.mu.Lock()
	s.notifier = n
	s.mu.Unlock()
	s.notifyMu.Unlock()
}

// NotificationClicked handles a click on a notification the service sent:
// it brings a window forward (WithShow) and emits desktop:show-tasks so the
// frontend selects the result. It reports false for a notification it
// didn't send, which it leaves to the app.
func (s *Service) NotificationClicked(id string) bool {
	s.mu.Lock()
	ids, ok := s.sent[id]
	show := s.show
	s.mu.Unlock()
	if !ok {
		return false
	}
	if show != nil {
		show()
	}
	s.emit(EventShowTasks, ShowTasksPayload{Tasks: append([]string(nil), ids...)})
	return true
}

// Running returns the number of top-level tasks that haven't finished.
func (s *Service) Running() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// ShouldQuit decides whether the app may quit now. Pass it as Wails's
// application.Options.ShouldQuit. With no unfinished work, or once the
// person has chosen Quit, it returns true. Otherwise it returns false and
// asks the quit question on another goroutine (it is called on the main
// thread, which the dialog needs); the answer quits, quits when the work
// finishes, or keeps running. Attach also calls it when the last window
// closes where that quits the app.
func (s *Service) ShouldQuit() bool {
	s.mu.Lock()
	if s.quitAllowed || s.running == 0 || s.asker == nil || s.closed {
		s.mu.Unlock()
		return true
	}
	if !s.asking {
		s.asking = true
		q := QuitQuestion{
			Message:      s.words.QuitMessage(s.running),
			Detail:       s.words.QuitDetail,
			Running:      s.running,
			KeepRunning:  s.words.KeepRunning,
			QuitWhenDone: s.words.QuitWhenDone,
			QuitNow:      s.words.QuitNow,
		}
		go s.ask(s.asker, q)
	}
	s.mu.Unlock()
	return false
}

func (s *Service) ask(asker Asker, q QuitQuestion) {
	choice := asker.AskQuit(q)
	s.mu.Lock()
	s.asking = false
	quit, pending := false, false
	switch choice {
	case QuitNow:
		s.quitAllowed, s.quitPending, quit = true, false, true
	case QuitWhenDone:
		if s.running == 0 {
			s.quitAllowed, quit = true, true
		} else {
			s.quitPending, pending = true, true
		}
	}
	running := s.running
	s.mu.Unlock()
	if pending {
		s.emit(EventQuitPending, QuitPendingPayload{Pending: true, Running: running})
	}
	if quit {
		s.callQuit()
	}
}

func (s *Service) callQuit() {
	s.mu.Lock()
	f := s.quit
	s.mu.Unlock()
	if f != nil {
		go f() // never from inside a tasks:changed handler or a dialog callback
	}
}

// QuitPending reports whether the app will quit once the work finishes.
func (s *Service) QuitPending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.quitPending
}

// CancelQuit takes back Quit When Done.
func (s *Service) CancelQuit() {
	s.mu.Lock()
	was := s.quitPending
	s.quitPending = false
	running := s.running
	s.mu.Unlock()
	if was {
		s.emit(EventQuitPending, QuitPendingPayload{Pending: false, Running: running})
	}
}

// Bindings returns the frontend's view of the service, for
// application.NewService: QuitPending and CancelQuit.
func (s *Service) Bindings() *Bindings { return &Bindings{svc: s} }

// Bindings is what the frontend may call. Bind it, not the Service.
type Bindings struct{ svc *Service }

// QuitPending reports whether the app will quit once the work finishes.
func (b *Bindings) QuitPending() bool { return b.svc.QuitPending() }

// CancelQuit takes back Quit When Done.
func (b *Bindings) CancelQuit() { b.svc.CancelQuit() }

// Close stops following tasks and drops notifications not yet sent.
func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.pending = nil
	if s.group != nil {
		s.group.Stop()
		s.group = nil
	}
	unsub := s.unsubscribe
	s.mu.Unlock()
	if unsub != nil {
		unsub()
	}
}
