package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// RecoverAndLog returns a function intended for use with defer in goroutines.
// If a panic occurs, it captures the stack trace and writes it to a crash log
// file in the service's log directory. The crash log is automatically included
// in the next bundle created by CreateBundle.
//
// Usage:
//
//	go func() {
//	    defer diagnostics.RecoverAndLog(diagSvc)()
//	    // ... work that may panic ...
//	}()
//
// Limitations:
//   - Only captures panics in goroutines that explicitly call this helper.
//   - Does not capture panics in the main goroutine or goroutines started by
//     third-party libraries.
func RecoverAndLog(svc *Service) func() {
	return func() {
		r := recover()
		if r == nil {
			return
		}

		logDir := svc.logDir
		if logDir == "" {
			return
		}

		stack := debug.Stack()
		timestamp := time.Now().UTC().Format("2006-01-02T15-04-05")
		filename := fmt.Sprintf("crash-%s.log", timestamp)

		content := fmt.Sprintf("panic: %v\n\n%s", r, stack)

		// Best-effort write; nothing to do if this fails.
		_ = os.MkdirAll(logDir, 0700)
		_ = os.WriteFile(filepath.Join(logDir, filename), []byte(content), 0600)
	}
}
