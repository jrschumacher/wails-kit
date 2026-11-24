//go:build (darwin || linux) && wails

package desktop

import "github.com/wailsapp/wails/v3/pkg/application"

// newPlatformTaskbar returns nil: macOS has no standard Dock progress (it
// needs a custom Dock tile view, which Wails doesn't expose), and Linux has
// no standard equivalent everywhere.
func newPlatformTaskbar(*application.App) Taskbar { return nil }

// darkBeforeRun can't tell on macOS and Linux: a window made before Run
// opens light and is repainted when the app starts.
func darkBeforeRun() (dark, known bool) { return false, false }
