// Package diagnostics collects application state, logs, and system info into
// a shareable zip bundle for crash reporting and user support.
package diagnostics

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/jrschumacher/wails-kit/appdirs"
	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
	"github.com/jrschumacher/wails-kit/settings"
)

// Error codes.
const (
	ErrBundleCreate errors.Code = "diagnostics_bundle"
	ErrBundleLogs   errors.Code = "diagnostics_logs"
)

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrBundleCreate: "Failed to create the diagnostics bundle. Please try again.",
		ErrBundleLogs:   "Failed to collect log files for the diagnostics bundle.",
	})
}

// Event names.
const (
	EventBundleCreated = "diagnostics:bundle_created"
)

// BundleCreatedPayload is emitted when a bundle is successfully created.
type BundleCreatedPayload struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// SystemInfo holds system and application metadata for the bundle.
type SystemInfo struct {
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	GoVersion  string    `json:"goVersion"`
	AppName    string    `json:"appName"`
	AppVersion string    `json:"appVersion"`
	NumCPU     int       `json:"numCPU"`
	Timestamp  time.Time `json:"timestamp"`
}

// Service creates diagnostics bundles from application state.
type Service struct {
	appName    string
	appVersion string
	logDir     string
	dirs       *appdirs.Dirs
	settings   *settings.Service
	emitter    *events.Emitter
	maxLogSize int64 // bytes; total cap for log files in bundle
}

// ServiceOption configures a Service.
type ServiceOption func(*Service)

// WithAppName sets the application name used in system info and bundle naming.
func WithAppName(name string) ServiceOption {
	return func(s *Service) { s.appName = name }
}

// WithVersion sets the application version included in system info.
func WithVersion(version string) ServiceOption {
	return func(s *Service) { s.appVersion = version }
}

// WithLogDir sets the directory to collect log files from.
// If not set but WithDirs is provided, the log directory is read from Dirs.
func WithLogDir(dir string) ServiceOption {
	return func(s *Service) { s.logDir = dir }
}

// WithDirs sets the appdirs.Dirs instance for resolving log paths.
func WithDirs(d *appdirs.Dirs) ServiceOption {
	return func(s *Service) { s.dirs = d }
}

// WithSettings connects a settings service for including sanitized settings.
func WithSettings(svc *settings.Service) ServiceOption {
	return func(s *Service) { s.settings = svc }
}

// WithEmitter sets the event emitter for diagnostics notifications.
func WithEmitter(e *events.Emitter) ServiceOption {
	return func(s *Service) { s.emitter = e }
}

// WithMaxLogSize sets the maximum total size (in bytes) of log files to
// include in the bundle. Defaults to 10MB.
func WithMaxLogSize(bytes int64) ServiceOption {
	return func(s *Service) { s.maxLogSize = bytes }
}

const defaultMaxLogSize = 10 * 1024 * 1024 // 10MB

// NewService creates a new diagnostics service.
func NewService(opts ...ServiceOption) (*Service, error) {
	s := &Service{
		maxLogSize: defaultMaxLogSize,
	}
	for _, opt := range opts {
		opt(s)
	}

	if s.appName == "" {
		return nil, fmt.Errorf("diagnostics: app name is required (use WithAppName)")
	}

	// Resolve log directory from Dirs if not explicitly set
	if s.logDir == "" && s.dirs != nil {
		s.logDir = s.dirs.Log()
	}

	return s, nil
}

// CreateBundle generates a diagnostics zip bundle at the given directory path.
// Returns the full path to the created zip file.
func (s *Service) CreateBundle(ctx context.Context, outputDir string) (string, error) {
	timestamp := time.Now().UTC()
	filename := fmt.Sprintf("diagnostics-%s-%s.zip",
		s.appName,
		timestamp.Format("2006-01-02T15-04-05"),
	)
	outputPath := filepath.Join(outputDir, filename)

	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return "", errors.Wrap(ErrBundleCreate, "create output directory", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return "", errors.Wrap(ErrBundleCreate, "create bundle file", err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	defer func() { _ = zw.Close() }()

	var manifest []string

	// 1. System info
	if err := s.writeSystemInfo(zw, timestamp); err != nil {
		return "", err
	}
	manifest = append(manifest, "system.json")

	// 2. Settings (sanitized)
	if s.settings != nil {
		if err := s.writeSettings(zw); err != nil {
			return "", err
		}
		manifest = append(manifest, "settings.json")
	}

	// 3. Log files
	if s.logDir != "" {
		logFiles, err := s.writeLogs(zw)
		if err != nil {
			return "", err
		}
		for _, lf := range logFiles {
			manifest = append(manifest, "logs/"+lf)
		}
	}

	// 4. Manifest
	if err := writeManifest(zw, manifest); err != nil {
		return "", errors.Wrap(ErrBundleCreate, "write manifest", err)
	}

	// Close the zip writer before getting file size
	if err := zw.Close(); err != nil {
		return "", errors.Wrap(ErrBundleCreate, "finalize zip", err)
	}

	// Get file size for the event
	fi, _ := f.Stat()
	var size int64
	if fi != nil {
		size = fi.Size()
	}

	s.emit(EventBundleCreated, BundleCreatedPayload{
		Path: outputPath,
		Size: size,
	})

	return outputPath, nil
}

// GetSystemInfo returns the current system info without creating a bundle.
// Useful for displaying in an "About" screen.
func (s *Service) GetSystemInfo() SystemInfo {
	return SystemInfo{
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		GoVersion:  runtime.Version(),
		AppName:    s.appName,
		AppVersion: s.appVersion,
		NumCPU:     runtime.NumCPU(),
		Timestamp:  time.Now().UTC(),
	}
}

func (s *Service) writeSystemInfo(zw *zip.Writer, timestamp time.Time) error {
	info := SystemInfo{
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		GoVersion:  runtime.Version(),
		AppName:    s.appName,
		AppVersion: s.appVersion,
		NumCPU:     runtime.NumCPU(),
		Timestamp:  timestamp,
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "marshal system info", err)
	}

	w, err := zw.Create("system.json")
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "create system.json in zip", err)
	}
	_, err = w.Write(data)
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "write system.json", err)
	}
	return nil
}

func (s *Service) writeSettings(zw *zip.Writer) error {
	values, err := s.settings.GetValues()
	if err != nil {
		// Non-fatal: include the error in the settings file instead
		values = map[string]any{"_error": err.Error()}
	}

	// Sanitize: redact password fields using the schema
	schema := s.settings.GetSchema()
	sanitized := sanitizeSettings(schema, values)

	data, err := json.MarshalIndent(sanitized, "", "  ")
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "marshal settings", err)
	}

	w, err := zw.Create("settings.json")
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "create settings.json in zip", err)
	}
	_, err = w.Write(data)
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "write settings.json", err)
	}
	return nil
}

// sanitizeSettings replaces password field values with "[REDACTED]".
func sanitizeSettings(schema settings.Schema, values map[string]any) map[string]any {
	passwordKeys := make(map[string]bool)
	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			if field.Type == settings.FieldPassword {
				passwordKeys[field.Key] = true
			}
		}
	}

	result := make(map[string]any, len(values))
	for k, v := range values {
		if passwordKeys[k] {
			result[k] = "[REDACTED]"
		} else {
			result[k] = v
		}
	}
	return result
}

// logEntry tracks a log file and its size for budget-aware collection.
type logEntry struct {
	path string
	name string
	size int64
}

func (s *Service) writeLogs(zw *zip.Writer) ([]string, error) {
	entries, err := os.ReadDir(s.logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(ErrBundleLogs, "read log directory", err)
	}

	// Collect log files (*.log, *.log.gz, *.log.*.gz)
	var logs []logEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isLogFile(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		logs = append(logs, logEntry{
			path: filepath.Join(s.logDir, name),
			name: name,
			size: info.Size(),
		})
	}

	// Sort by modification time descending (newest first) so we prioritize
	// recent logs when applying the size cap
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].name > logs[j].name
	})

	var totalSize int64
	var included []string

	for _, log := range logs {
		if totalSize+log.size > s.maxLogSize {
			continue
		}

		if err := addFileToZip(zw, "logs/"+log.name, log.path); err != nil {
			continue // skip unreadable files
		}

		totalSize += log.size
		included = append(included, log.name)
	}

	return included, nil
}

// isLogFile returns true for files that look like log files.
func isLogFile(name string) bool {
	// Match: app.log, app-2026-03-07.log, app.log.gz, app-2026-03-07.log.gz
	if strings.HasSuffix(name, ".log") {
		return true
	}
	if strings.HasSuffix(name, ".log.gz") {
		return true
	}
	return false
}

func addFileToZip(zw *zip.Writer, zipPath, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	w, err := zw.Create(zipPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	return err
}

func writeManifest(zw *zip.Writer, files []string) error {
	w, err := zw.Create("manifest.txt")
	if err != nil {
		return err
	}
	content := strings.Join(files, "\n") + "\n"
	_, err = w.Write([]byte(content))
	return err
}

func (s *Service) emit(name string, data any) {
	if s.emitter != nil {
		s.emitter.Emit(name, data)
	}
}
