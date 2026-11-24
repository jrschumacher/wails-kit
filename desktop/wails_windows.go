//go:build windows && wails

package desktop

import "github.com/wailsapp/wails/v3/pkg/w32"

// darkBeforeRun reads the system's appearance from the registry
// (HKCU\...\Themes\Personalize\AppsUseLightTheme), which Wails's own
// IsDarkMode can't do before app.Run.
func darkBeforeRun() (dark, known bool) { return w32.IsCurrentlyDarkMode(), true }
