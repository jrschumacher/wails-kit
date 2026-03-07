package logging

import (
	"os"
	"path/filepath"
	"runtime"
)

// logDir returns the OS-appropriate log directory for the given app name.
//   - macOS: ~/Library/Logs/{appName}/
//   - Linux: $XDG_STATE_HOME/{appName}/ (falls back to ~/.local/state/{appName}/)
//   - Windows: %LOCALAPPDATA%/{appName}/logs/
func logDir(appName string) string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Logs", appName)
	case "windows":
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, appName, "logs")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "AppData", "Local", appName, "logs")
	default: // linux and others
		if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
			return filepath.Join(dir, appName)
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "state", appName)
	}
}
