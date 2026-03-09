// Package appdirs provides OS-standard application directory paths for config,
// data, cache, log, and temp categories. It replaces duplicated path logic
// across the wails-kit packages with a single, consistent implementation.
package appdirs

import (
	"os"
	"path/filepath"
	"runtime"
)

// Dirs holds resolved application directory paths for all categories.
type Dirs struct {
	appName  string
	config   string
	data     string
	cache    string
	log      string
	temp     string
}

// Option configures a Dirs instance.
type Option func(*Dirs)

// WithConfigDir overrides the default config directory.
func WithConfigDir(path string) Option {
	return func(d *Dirs) { d.config = path }
}

// WithDataDir overrides the default data directory.
func WithDataDir(path string) Option {
	return func(d *Dirs) { d.data = path }
}

// WithCacheDir overrides the default cache directory.
func WithCacheDir(path string) Option {
	return func(d *Dirs) { d.cache = path }
}

// WithLogDir overrides the default log directory.
func WithLogDir(path string) Option {
	return func(d *Dirs) { d.log = path }
}

// WithTempDir overrides the default temp directory.
func WithTempDir(path string) Option {
	return func(d *Dirs) { d.temp = path }
}

// New creates a Dirs instance for the given app name with OS-appropriate defaults.
// Panics if appName is empty.
func New(appName string, opts ...Option) *Dirs {
	if appName == "" {
		panic("appdirs: appName must not be empty")
	}

	d := &Dirs{appName: appName}
	for _, opt := range opts {
		opt(d)
	}

	// Fill in OS defaults for any paths not overridden
	if d.config == "" {
		d.config = defaultConfig(appName)
	}
	if d.data == "" {
		d.data = defaultData(appName)
	}
	if d.cache == "" {
		d.cache = defaultCache(appName)
	}
	if d.log == "" {
		d.log = defaultLog(appName)
	}
	if d.temp == "" {
		d.temp = filepath.Join(os.TempDir(), appName)
	}

	return d
}

// Config returns the directory for settings and preferences.
//   - macOS: ~/Library/Application Support/{app}/
//   - Linux: $XDG_CONFIG_HOME/{app}/ (default ~/.config/{app}/)
//   - Windows: %APPDATA%/{app}/
func (d *Dirs) Config() string { return d.config }

// Data returns the directory for persistent user data (databases, user content).
//   - macOS: ~/Library/Application Support/{app}/
//   - Linux: $XDG_DATA_HOME/{app}/ (default ~/.local/share/{app}/)
//   - Windows: %APPDATA%/{app}/
func (d *Dirs) Data() string { return d.data }

// Cache returns the directory for ephemeral cached data.
//   - macOS: ~/Library/Caches/{app}/
//   - Linux: $XDG_CACHE_HOME/{app}/ (default ~/.cache/{app}/)
//   - Windows: %LOCALAPPDATA%/{app}/cache/
func (d *Dirs) Cache() string { return d.cache }

// Log returns the directory for log files.
//   - macOS: ~/Library/Logs/{app}/
//   - Linux: $XDG_STATE_HOME/{app}/ (default ~/.local/state/{app}/)
//   - Windows: %LOCALAPPDATA%/{app}/logs/
func (d *Dirs) Log() string { return d.log }

// Temp returns the directory for temporary working files.
//   - All platforms: {os.TempDir()}/{app}/
func (d *Dirs) Temp() string { return d.temp }

// EnsureAll creates all directories with 0700 permissions.
func (d *Dirs) EnsureAll() error {
	for _, dir := range []string{d.config, d.data, d.cache, d.log, d.temp} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return nil
}

// CleanTemp removes all contents of the temp directory and recreates it empty.
// Does not error if the temp directory does not exist.
func (d *Dirs) CleanTemp() error {
	if err := os.RemoveAll(d.temp); err != nil {
		return err
	}
	return os.MkdirAll(d.temp, 0700)
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

func defaultConfig(appName string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir(), "Library", "Application Support", appName)
	case "windows":
		if dir := os.Getenv("APPDATA"); dir != "" {
			return filepath.Join(dir, appName)
		}
		return filepath.Join(homeDir(), "AppData", "Roaming", appName)
	default:
		if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
			return filepath.Join(dir, appName)
		}
		return filepath.Join(homeDir(), ".config", appName)
	}
}

func defaultData(appName string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir(), "Library", "Application Support", appName)
	case "windows":
		if dir := os.Getenv("APPDATA"); dir != "" {
			return filepath.Join(dir, appName)
		}
		return filepath.Join(homeDir(), "AppData", "Roaming", appName)
	default:
		if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
			return filepath.Join(dir, appName)
		}
		return filepath.Join(homeDir(), ".local", "share", appName)
	}
}

func defaultCache(appName string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir(), "Library", "Caches", appName)
	case "windows":
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, appName, "cache")
		}
		return filepath.Join(homeDir(), "AppData", "Local", appName, "cache")
	default:
		if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
			return filepath.Join(dir, appName)
		}
		return filepath.Join(homeDir(), ".cache", appName)
	}
}

func defaultLog(appName string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir(), "Library", "Logs", appName)
	case "windows":
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, appName, "logs")
		}
		return filepath.Join(homeDir(), "AppData", "Local", appName, "logs")
	default:
		if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
			return filepath.Join(dir, appName)
		}
		return filepath.Join(homeDir(), ".local", "state", appName)
	}
}
