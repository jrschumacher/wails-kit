//go:build (darwin || linux || windows) && wails

package desktop

import (
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// RGBA returns the colour as Wails takes it, opaque.
func (c Colour) RGBA() application.RGBA {
	return application.NewRGBA(c.R, c.G, c.B, 255)
}

// WindowBackground is the background for a new window's options
// (WebviewWindowOptions.BackgroundColour), so the window opens in the
// theme's bg for the current appearance before the web view paints. Attach
// keeps it in step afterwards.
func (s *Service) WindowBackground(app *application.App) application.RGBA {
	return s.Background(app.Env.IsDarkMode()).RGBA()
}

// Attach wires the service to a Wails app. Call it after application.New
// and before app.Run. It:
//
//   - follows every window's focus, so notifications and the taskbar's
//     Error state know whether the person is watching;
//   - asks the quit question when the last window closes, where that quits
//     the app (pass ShouldQuit as application.Options.ShouldQuit for the
//     other ways to quit);
//   - paints every window's background with the theme's bg, and again
//     when the system appearance changes;
//   - draws taskbar progress on Windows through ITaskbarList3;
//   - routes Windows's sleep and wake events to the tasks service's Sleep
//     and Wake (WithTasks).
//
// It fills in what wasn't set with options: the quit function (app.Quit),
// the window to bring forward on a notification click, the notification
// title (the app's name), the quit question (a Wails question dialog) and
// the taskbar.
func (s *Service) Attach(app *application.App) {
	s.mu.Lock()
	if s.quit == nil {
		s.quit = app.Quit
	}
	if s.show == nil {
		s.show = func() { bringForward(app) }
	}
	if s.appName == "" {
		s.appName = app.Config().Name
	}
	if s.asker == nil {
		s.asker = dialogAsker{app: app}
	}
	s.onAppearance = func() { s.paintAll(app) }
	s.mu.Unlock()

	s.osMu.Lock()
	if s.taskbar == nil {
		if t := newPlatformTaskbar(app); t != nil {
			s.taskbar = t
		}
	}
	s.osMu.Unlock()

	f := &focusSet{ids: map[uint]bool{}}
	for _, w := range app.Window.GetAll() {
		s.watch(app, w, f)
	}
	app.Window.OnCreate(func(w application.Window) { s.watch(app, w, f) })
	app.Event.OnApplicationEvent(events.Common.ThemeChanged, func(*application.ApplicationEvent) { s.paintAll(app) })
	if s.tasks != nil {
		app.Event.OnApplicationEvent(events.Windows.APMSuspend, func(*application.ApplicationEvent) { s.tasks.Sleep() })
		app.Event.OnApplicationEvent(events.Windows.APMResumeAutomatic, func(*application.ApplicationEvent) { s.tasks.Wake() })
	}
	s.apply()
}

// focusSet tracks which windows are focused.
type focusSet struct {
	mu  sync.Mutex
	ids map[uint]bool
}

func (f *focusSet) set(id uint, focused bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if focused {
		f.ids[id] = true
	} else {
		delete(f.ids, id)
	}
	return len(f.ids) > 0
}

func (s *Service) watch(app *application.App, w application.Window, f *focusSet) {
	id := w.ID()
	w.OnWindowEvent(events.Common.WindowFocus, func(*application.WindowEvent) { s.SetFocused(f.set(id, true)) })
	w.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) { s.SetFocused(f.set(id, false)) })
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { s.SetFocused(f.set(id, false)) })
	w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if quitsOnLastWindow(app) && len(app.Window.GetAll()) <= 1 && !s.ShouldQuit() {
			e.Cancel()
		}
	})
	w.SetBackgroundColour(s.WindowBackground(app))
	// A window can be created focused, before its focus event is hooked.
	if w.IsFocused() {
		s.SetFocused(f.set(id, true))
	}
}

func (s *Service) paintAll(app *application.App) {
	bg := s.WindowBackground(app)
	for _, w := range app.Window.GetAll() {
		w.SetBackgroundColour(bg)
	}
}

// quitsOnLastWindow reports whether closing the last window quits the app,
// by the app's Wails options.
func quitsOnLastWindow(app *application.App) bool {
	cfg := app.Config()
	switch runtime.GOOS {
	case "darwin":
		return cfg.Mac.ApplicationShouldTerminateAfterLastWindowClosed
	case "windows":
		return !cfg.Windows.DisableQuitOnLastWindowClosed
	default:
		return !cfg.Linux.DisableQuitOnLastWindowClosed
	}
}

// bringForward restores, shows and focuses the current window, or else the
// first one.
func bringForward(app *application.App) {
	w := app.Window.Current()
	if w == nil {
		all := app.Window.GetAll()
		if len(all) == 0 {
			return
		}
		w = all[0]
	}
	if w.IsMinimised() {
		w.Restore()
	}
	w.Show()
	w.Focus()
}

// linuxUnanswered is how long the Linux quit question waits for a button
// before it counts as Keep Running: a GTK dialog closed with Esc runs no
// button's callback.
const linuxUnanswered = time.Minute

// dialogAsker asks the quit question with a Wails question dialog.
//
// Wails's Windows question dialog is a MessageBox with Yes and No only, so
// there it asks "Quit now?" without Quit When Done: Yes quits, No (the
// default) keeps running. An app that wants all three choices on Windows
// sets its own Asker (WithAsker), for example a sheet in the window.
type dialogAsker struct{ app *application.App }

func (d dialogAsker) AskQuit(q QuitQuestion) QuitChoice {
	answer := make(chan QuitChoice, 1)
	pick := func(c QuitChoice) func() {
		return func() {
			select {
			case answer <- c:
			default:
			}
		}
	}
	dialog := d.app.Dialog.Question()
	if runtime.GOOS == "windows" {
		// The detail names Quit When Done, which a MessageBox can't offer.
		dialog.SetTitle(d.app.Config().Name).SetMessage(q.Message)
		dialog.AddButton("Yes").OnClick(pick(QuitNow))
		dialog.AddButton("No").SetAsDefault().SetAsCancel().OnClick(pick(QuitKeepRunning))
		dialog.Show() // synchronous on Windows
		select {
		case c := <-answer:
			return c
		default:
			return QuitKeepRunning
		}
	}
	dialog.SetTitle(q.Message).SetMessage(q.Detail)
	dialog.AddButton(q.KeepRunning).SetAsDefault().SetAsCancel().OnClick(pick(QuitKeepRunning))
	dialog.AddButton(q.QuitWhenDone).OnClick(pick(QuitWhenDone))
	dialog.AddButton(q.QuitNow).OnClick(pick(QuitNow))
	dialog.Show()
	if runtime.GOOS == "linux" {
		select {
		case c := <-answer:
			return c
		case <-time.After(linuxUnanswered):
			return QuitKeepRunning
		}
	}
	return <-answer
}

// UseNotifications sends the service's notifications through Wails's
// notifications service, and routes clicks on them to NotificationClicked.
// Register ns with the app (application.NewService(ns)) as usual. Wails
// keeps one response callback, so this replaces any the app set; pass the
// handler for the app's own notifications as other.
func (s *Service) UseNotifications(ns *notifications.NotificationService, other func(notifications.NotificationResult)) {
	ns.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error == nil && s.NotificationClicked(r.Response.ID) {
			return
		}
		if other != nil {
			other(r)
		}
	})
	s.SetNotifier(wailsNotifier{ns: ns})
}

type wailsNotifier struct {
	ns *notifications.NotificationService
}

func (w wailsNotifier) Authorized() (bool, error) { return w.ns.CheckNotificationAuthorization() }

func (w wailsNotifier) RequestAuthorization() (bool, error) {
	return w.ns.RequestNotificationAuthorization()
}

func (w wailsNotifier) Notify(n Notification) error {
	return w.ns.SendNotification(notifications.NotificationOptions{ID: n.ID, Title: n.Title, Body: n.Body})
}
