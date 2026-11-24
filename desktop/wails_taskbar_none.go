//go:build windows && wails && !amd64 && !arm64

package desktop

import "github.com/wailsapp/wails/v3/pkg/application"

// newPlatformTaskbar returns nil on 32-bit Windows: SetProgressValue takes
// two 64-bit values, which the vtable call here passes as one word each.
func newPlatformTaskbar(*application.App) Taskbar { return nil }
