// Package desktop shows an app's long work where a desktop puts it outside
// the window: a system notification when long work ends while the window
// isn't focused, progress on the Windows taskbar button, a badge with the
// count of things that need the person, a question before quitting while
// work runs, and each window's background colour before the web view paints.
//
// It implements the "Desktop" section of frontend/ui/BEHAVIOUR.md. It
// follows the tasks package's tasks:changed snapshots, so the app reports
// its work once, to tasks, and this package draws it on the desktop.
//
// The rules live here and are tested with fakes; the operating system is
// reached through small interfaces (Notifier, Badge, Taskbar, Asker). The
// Wails adapters (Attach, UseNotifications) are behind the "wails" build tag,
// like the shortcuts package's Apply.
package desktop

import (
	"fmt"
	"strings"
	"time"

	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/tasks"
)

// Events the service emits.
const (
	// EventShowTasks asks the frontend to select the tasks a clicked
	// notification was about. Payload: ShowTasksPayload.
	EventShowTasks = "desktop:show-tasks"
	// EventQuitPending says whether the app will quit once the work
	// finishes (the person chose "Quit When Done"). Payload:
	// QuitPendingPayload.
	EventQuitPending = "desktop:quit-pending"
)

// ShowTasksPayload is the payload of desktop:show-tasks: the IDs of the
// top-level tasks a notification was about, in the order they finished.
type ShowTasksPayload struct {
	Tasks []string `json:"tasks"`
}

// QuitPendingPayload is the payload of desktop:quit-pending. Running is the
// number of top-level tasks that haven't finished.
type QuitPendingPayload struct {
	Pending bool `json:"pending"`
	Running int  `json:"running"`
}

// Defaults, from BEHAVIOUR.md's Desktop section.
const (
	// DefaultLongWork is how long work must have run for its end to be
	// worth a notification.
	DefaultLongWork = 10 * time.Second
	// DefaultGroupWindow is how long the service waits after a task ends
	// for others to end with it, so that they share one notification.
	DefaultGroupWindow = time.Second
)

// Notification is one system notification. Title is the app's name; Body
// is one full sentence.
type Notification struct {
	ID    string
	Title string
	Body  string
}

// Notifier sends system notifications. UseNotifications adapts Wails's
// notifications service (build tag "wails"); tests use a fake.
type Notifier interface {
	// Authorized reports whether the app may send notifications now,
	// without asking.
	Authorized() (bool, error)
	// RequestAuthorization asks the person for permission. The service
	// calls it at most once per run, the first time a notification would
	// help, never at launch.
	RequestAuthorization() (bool, error)
	// Notify sends the notification.
	Notify(n Notification) error
}

// Badge shows a count on the app's icon. Wails's dock service
// (github.com/wailsapp/wails/v3/pkg/services/dock) has exactly these
// methods: a badge on macOS, an overlay icon on Windows, nothing on Linux.
type Badge interface {
	SetBadge(label string) error
	RemoveBadge() error
}

// TaskbarState is a progress state of the Windows taskbar button. The
// values are ITaskbarList3's TBPF_* flags. Paused (yellow) is left out on
// purpose: paused work shows in the window's words.
type TaskbarState int

// The taskbar states the kit uses.
const (
	TaskbarNone          TaskbarState = 0 // TBPF_NOPROGRESS
	TaskbarIndeterminate TaskbarState = 1 // TBPF_INDETERMINATE
	TaskbarNormal        TaskbarState = 2 // TBPF_NORMAL
	TaskbarError         TaskbarState = 4 // TBPF_ERROR
)

func (s TaskbarState) String() string {
	switch s {
	case TaskbarNone:
		return "none"
	case TaskbarIndeterminate:
		return "indeterminate"
	case TaskbarNormal:
		return "normal"
	case TaskbarError:
		return "error"
	}
	return fmt.Sprintf("TaskbarState(%d)", int(s))
}

// Taskbar draws progress on the app's taskbar button. On Windows, Attach
// provides one through ITaskbarList3; elsewhere there is none.
type Taskbar interface {
	SetProgressState(state TaskbarState) error
	SetProgressValue(completed, total uint64) error
}

// QuitChoice is the person's answer to the quit question.
type QuitChoice int

// The answers. QuitKeepRunning is the safe default.
const (
	QuitKeepRunning QuitChoice = iota
	QuitWhenDone
	QuitNow
)

// QuitQuestion is what the quit question shows. Buttons are in order, the
// safe choice (Keep Running) first: it is the default and the cancel
// button.
type QuitQuestion struct {
	Message      string
	Detail       string
	Running      int
	KeepRunning  string
	QuitWhenDone string
	QuitNow      string
}

// Asker asks the quit question. AskQuit blocks until the person answers;
// the service calls it on its own goroutine, never on the thread that asked
// to quit. Attach provides one built on a Wails question dialog.
type Asker interface {
	AskQuit(q QuitQuestion) QuitChoice
}

// AskerFunc adapts a function to Asker.
type AskerFunc func(q QuitQuestion) QuitChoice

// AskQuit calls f.
func (f AskerFunc) AskQuit(q QuitQuestion) QuitChoice { return f(q) }

// Words are the service's words. The defaults are English; an app sets its
// own with WithWords. Notification words leave the app: they go to the
// system's notification centre, so they must hold no secrets and no
// private content (a token, a private repository's code, a commit
// message).
type Words struct {
	// Notification is the body of a notification about the given
	// top-level tasks, which finished together: one full sentence. The
	// default uses each task's own words for one task ("Report exported."),
	// and counts for several ("3 tasks finished.", "2 tasks finished and
	// 1 failed."). It never uses a task's subject or raw error.
	Notification func(finished []tasks.Snapshot) string
	// QuitMessage asks the quit question: "Quit while 2 tasks are
	// running?".
	QuitMessage func(running int) string
	// QuitDetail explains the choice, under the message.
	QuitDetail string
	// Button labels.
	KeepRunning  string
	QuitWhenDone string
	QuitNow      string
}

// DefaultWords returns the English words.
func DefaultWords() Words {
	return Words{
		Notification: defaultNotification,
		QuitMessage: func(n int) string {
			if n == 1 {
				return "Quit while 1 task is running?"
			}
			return fmt.Sprintf("Quit while %d tasks are running?", n)
		},
		QuitDetail:   "Quitting stops the work. Quit When Done waits for it to finish.",
		KeepRunning:  "Keep Running",
		QuitWhenDone: "Quit When Done",
		QuitNow:      "Quit",
	}
}

func defaultNotification(finished []tasks.Snapshot) string {
	if len(finished) == 1 {
		t := finished[0]
		if w := sentence(t.Words); w != "" {
			return w
		}
		if t.State == tasks.StateFailed {
			return "A task failed."
		}
		return "A task finished."
	}
	failed := 0
	for _, t := range finished {
		if t.State == tasks.StateFailed {
			failed++
		}
	}
	done := len(finished) - failed
	switch {
	case failed == 0:
		return fmt.Sprintf("%d tasks finished.", done)
	case done == 0:
		return fmt.Sprintf("%d tasks failed.", failed)
	case done == 1:
		return fmt.Sprintf("1 task finished and %d failed.", failed)
	default:
		return fmt.Sprintf("%d tasks finished and %d failed.", done, failed)
	}
}

// sentence makes words a sentence: trimmed, with a full stop unless they
// already end in punctuation. The kit's words join a hint with " · "
// ("Couldn't save the file · Retry"); a notification keeps the part before
// it, since its buttons aren't there.
func sentence(words string) string {
	if i := strings.Index(words, " · "); i >= 0 {
		words = words[:i]
	}
	words = strings.TrimSpace(words)
	if words == "" {
		return ""
	}
	switch words[len(words)-1] {
	case '.', '!', '?':
		return words
	}
	if strings.HasSuffix(words, "…") {
		return words
	}
	return words + "."
}

// Error codes.
const (
	ErrInvalidColour errors.Code = "desktop_invalid_colour"
)

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrInvalidColour: "The colour isn't a #rrggbb value",
	})
}
