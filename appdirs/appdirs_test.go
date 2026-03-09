package appdirs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNew(t *testing.T) {
	dirs := New("test-app")
	if dirs.appName != "test-app" {
		t.Fatalf("expected appName test-app, got %s", dirs.appName)
	}
}

func TestNewPanicsOnEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty app name")
		}
	}()
	New("")
}

func TestNewPanicsOnNoHome(t *testing.T) {
	// Unsetting HOME should cause os.UserHomeDir() to fail
	t.Setenv("HOME", "")
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", "")
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when home directory cannot be resolved")
		}
	}()
	New("test-app")
}

func TestNewNoHomeWithAllOverrides(t *testing.T) {
	// With all dirs overridden, home dir is not needed — should not panic
	t.Setenv("HOME", "")
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", "")
	}

	dirs := New("test-app",
		WithConfigDir("/c"),
		WithDataDir("/d"),
		WithCacheDir("/ca"),
		WithLogDir("/l"),
		WithTempDir("/t"),
	)
	if dirs.Config() != "/c" {
		t.Fatalf("expected /c, got %s", dirs.Config())
	}
}

func TestConfig(t *testing.T) {
	dirs := New("test-app")
	path := dirs.Config()

	if path == "" {
		t.Fatal("Config() returned empty string")
	}
	if filepath.Base(filepath.Dir(path)) != "" && !filepath.IsAbs(path) {
		t.Fatalf("Config() should return absolute path, got %s", path)
	}

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, "Library", "Application Support", "test-app")
		if path != expected {
			t.Fatalf("expected %s, got %s", expected, path)
		}
	case "windows":
		if dir := os.Getenv("APPDATA"); dir != "" {
			expected := filepath.Join(dir, "test-app")
			if path != expected {
				t.Fatalf("expected %s, got %s", expected, path)
			}
		}
	default:
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".config", "test-app")
		// Could be overridden by XDG_CONFIG_HOME
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			expected = filepath.Join(xdg, "test-app")
		}
		if path != expected {
			t.Fatalf("expected %s, got %s", expected, path)
		}
	}
}

func TestData(t *testing.T) {
	dirs := New("test-app")
	path := dirs.Data()

	if path == "" {
		t.Fatal("Data() returned empty string")
	}

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, "Library", "Application Support", "test-app")
		if path != expected {
			t.Fatalf("expected %s, got %s", expected, path)
		}
	case "windows":
		if dir := os.Getenv("APPDATA"); dir != "" {
			expected := filepath.Join(dir, "test-app")
			if path != expected {
				t.Fatalf("expected %s, got %s", expected, path)
			}
		}
	default:
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".local", "share", "test-app")
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			expected = filepath.Join(xdg, "test-app")
		}
		if path != expected {
			t.Fatalf("expected %s, got %s", expected, path)
		}
	}
}

func TestCache(t *testing.T) {
	dirs := New("test-app")
	path := dirs.Cache()

	if path == "" {
		t.Fatal("Cache() returned empty string")
	}

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, "Library", "Caches", "test-app")
		if path != expected {
			t.Fatalf("expected %s, got %s", expected, path)
		}
	case "windows":
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			expected := filepath.Join(dir, "test-app", "cache")
			if path != expected {
				t.Fatalf("expected %s, got %s", expected, path)
			}
		}
	default:
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".cache", "test-app")
		if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
			expected = filepath.Join(xdg, "test-app")
		}
		if path != expected {
			t.Fatalf("expected %s, got %s", expected, path)
		}
	}
}

func TestLog(t *testing.T) {
	dirs := New("test-app")
	path := dirs.Log()

	if path == "" {
		t.Fatal("Log() returned empty string")
	}

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, "Library", "Logs", "test-app")
		if path != expected {
			t.Fatalf("expected %s, got %s", expected, path)
		}
	case "windows":
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			expected := filepath.Join(dir, "test-app", "logs")
			if path != expected {
				t.Fatalf("expected %s, got %s", expected, path)
			}
		}
	default:
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".local", "state", "test-app")
		if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
			expected = filepath.Join(xdg, "test-app")
		}
		if path != expected {
			t.Fatalf("expected %s, got %s", expected, path)
		}
	}
}

func TestTemp(t *testing.T) {
	dirs := New("test-app")
	path := dirs.Temp()

	if path == "" {
		t.Fatal("Temp() returned empty string")
	}

	expected := filepath.Join(os.TempDir(), "test-app")
	if path != expected {
		t.Fatalf("expected %s, got %s", expected, path)
	}
}

func TestWithOverrides(t *testing.T) {
	dirs := New("test-app",
		WithConfigDir("/custom/config"),
		WithDataDir("/custom/data"),
		WithCacheDir("/custom/cache"),
		WithLogDir("/custom/log"),
		WithTempDir("/custom/temp"),
	)

	if dirs.Config() != "/custom/config" {
		t.Fatalf("expected /custom/config, got %s", dirs.Config())
	}
	if dirs.Data() != "/custom/data" {
		t.Fatalf("expected /custom/data, got %s", dirs.Data())
	}
	if dirs.Cache() != "/custom/cache" {
		t.Fatalf("expected /custom/cache, got %s", dirs.Cache())
	}
	if dirs.Log() != "/custom/log" {
		t.Fatalf("expected /custom/log, got %s", dirs.Log())
	}
	if dirs.Temp() != "/custom/temp" {
		t.Fatalf("expected /custom/temp, got %s", dirs.Temp())
	}
}

func TestEnsureAll(t *testing.T) {
	base := t.TempDir()
	dirs := New("test-app",
		WithConfigDir(filepath.Join(base, "config")),
		WithDataDir(filepath.Join(base, "data")),
		WithCacheDir(filepath.Join(base, "cache")),
		WithLogDir(filepath.Join(base, "log")),
		WithTempDir(filepath.Join(base, "temp")),
	)

	if err := dirs.EnsureAll(); err != nil {
		t.Fatalf("EnsureAll() error: %v", err)
	}

	for _, name := range []string{"config", "data", "cache", "log", "temp"} {
		path := filepath.Join(base, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("directory %s not created: %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", name)
		}
		if runtime.GOOS != "windows" {
			if perm := info.Mode().Perm(); perm != 0700 {
				t.Fatalf("%s has permissions %o, expected 0700", name, perm)
			}
		}
	}
}

func TestCleanTemp(t *testing.T) {
	base := t.TempDir()
	tempDir := filepath.Join(base, "temp")
	dirs := New("test-app", WithTempDir(tempDir))

	// Create temp dir with some files
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "stale.tmp"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(tempDir, "subdir")
	if err := os.MkdirAll(subDir, 0700); err != nil {
		t.Fatal(err)
	}

	if err := dirs.CleanTemp(); err != nil {
		t.Fatalf("CleanTemp() error: %v", err)
	}

	// Temp dir itself should still exist (recreated empty)
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("temp dir should exist after CleanTemp: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temp dir should be empty, has %d entries", len(entries))
	}
}

func TestCleanTempNoDir(t *testing.T) {
	// Use a path under t.TempDir() that doesn't exist yet
	base := t.TempDir()
	dirs := New("test-app", WithTempDir(filepath.Join(base, "nonexistent", "deep")))

	// CleanTemp on nonexistent dir should not error
	if err := dirs.CleanTemp(); err != nil {
		t.Fatalf("CleanTemp() on nonexistent dir should not error: %v", err)
	}
}

func TestPartialOverride(t *testing.T) {
	dirs := New("test-app", WithConfigDir("/custom/config"))

	if dirs.Config() != "/custom/config" {
		t.Fatalf("expected /custom/config, got %s", dirs.Config())
	}
	// Other dirs should use OS defaults, not be empty
	if dirs.Data() == "" {
		t.Fatal("Data() should return OS default, not empty")
	}
	if dirs.Cache() == "" {
		t.Fatal("Cache() should return OS default, not empty")
	}
}

func TestConfigDirDarwin(t *testing.T) {
	got := configDir("darwin", "/Users/test", "my-app")
	expected := filepath.Join("/Users/test", "Library", "Application Support", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestConfigDirLinuxDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	got := configDir("linux", "/home/test", "my-app")
	expected := filepath.Join("/home/test", ".config", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestConfigDirLinuxXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	got := configDir("linux", "/home/test", "my-app")
	expected := filepath.Join("/custom/config", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestConfigDirWindowsEnv(t *testing.T) {
	t.Setenv("APPDATA", "/appdata")
	got := configDir("windows", "/home/test", "my-app")
	expected := filepath.Join("/appdata", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestConfigDirWindowsFallback(t *testing.T) {
	t.Setenv("APPDATA", "")
	got := configDir("windows", "/home/test", "my-app")
	expected := filepath.Join("/home/test", "AppData", "Roaming", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestDataDirDarwin(t *testing.T) {
	got := dataDir("darwin", "/Users/test", "my-app")
	expected := filepath.Join("/Users/test", "Library", "Application Support", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestDataDirLinuxDefault(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	got := dataDir("linux", "/home/test", "my-app")
	expected := filepath.Join("/home/test", ".local", "share", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestDataDirLinuxXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	got := dataDir("linux", "/home/test", "my-app")
	expected := filepath.Join("/custom/data", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestDataDirWindowsEnv(t *testing.T) {
	t.Setenv("APPDATA", "/appdata")
	got := dataDir("windows", "/home/test", "my-app")
	expected := filepath.Join("/appdata", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestDataDirWindowsFallback(t *testing.T) {
	t.Setenv("APPDATA", "")
	got := dataDir("windows", "/home/test", "my-app")
	expected := filepath.Join("/home/test", "AppData", "Roaming", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestCacheDirDarwin(t *testing.T) {
	got := cacheDir("darwin", "/Users/test", "my-app")
	expected := filepath.Join("/Users/test", "Library", "Caches", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestCacheDirLinuxDefault(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	got := cacheDir("linux", "/home/test", "my-app")
	expected := filepath.Join("/home/test", ".cache", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestCacheDirLinuxXDG(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")
	got := cacheDir("linux", "/home/test", "my-app")
	expected := filepath.Join("/custom/cache", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestCacheDirWindowsEnv(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "/localappdata")
	got := cacheDir("windows", "/home/test", "my-app")
	expected := filepath.Join("/localappdata", "my-app", "cache")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestCacheDirWindowsFallback(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	got := cacheDir("windows", "/home/test", "my-app")
	expected := filepath.Join("/home/test", "AppData", "Local", "my-app", "cache")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestLogDirDarwin(t *testing.T) {
	got := logDir("darwin", "/Users/test", "my-app")
	expected := filepath.Join("/Users/test", "Library", "Logs", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestLogDirLinuxDefault(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	got := logDir("linux", "/home/test", "my-app")
	expected := filepath.Join("/home/test", ".local", "state", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestLogDirLinuxXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	got := logDir("linux", "/home/test", "my-app")
	expected := filepath.Join("/custom/state", "my-app")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestLogDirWindowsEnv(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "/localappdata")
	got := logDir("windows", "/home/test", "my-app")
	expected := filepath.Join("/localappdata", "my-app", "logs")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestLogDirWindowsFallback(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	got := logDir("windows", "/home/test", "my-app")
	expected := filepath.Join("/home/test", "AppData", "Local", "my-app", "logs")
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}
