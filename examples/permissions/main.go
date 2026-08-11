// Command permissions-example demonstrates the permissions.Service state
// machine: Check reporting one of granted/denied/not_determined/unsupported
// for each Kind, and — only when explicitly asked for via a flag, since
// they can show a real OS dialog or open System Settings — Request and
// OpenSystemSettings.
//
// It compiles and vets headlessly on every platform (no build tag — AD-4).
// Running it with no flags is always safe and non-interactive: it only
// calls Check, which never prompts. On non-darwin it reports Unsupported
// for everything (platform_other.go); on darwin it reports real state, but
// notifications' Check requires a signed, bundled app to return anything
// but an SDK error (surfaced here as Unsupported too) — see
// permissions/README.md and AGENTS.md for exactly what this example (and
// permissions/'s own test suite) cannot demonstrate.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/jrschumacher/wails-kit/v2/permissions"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	requestKind := flag.String("request", "", "call Request for this Kind (notifications|accessibility|full_disk_access) — may show a real OS dialog")
	openKind := flag.String("open", "", "call OpenSystemSettings for this Kind — opens a real System Settings pane")
	flag.Parse()

	svc := permissions.NewService()

	kinds := []permissions.Kind{
		permissions.Notifications,
		permissions.Accessibility,
		permissions.FullDiskAccess,
	}

	fmt.Println("current state (Check never prompts):")
	for _, k := range kinds {
		printStatus(svc, k)
	}

	if *requestKind != "" {
		k := permissions.Kind(*requestKind)
		fmt.Printf("\nrequesting %s (10s timeout — Request never blocks past ctx, "+
			"even if the OS dialog is still open when it expires)...\n", k)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		status, err := svc.Request(ctx, k)
		if err != nil {
			fmt.Printf("  Request returned an error (often just ctx.DeadlineExceeded if you didn't answer the dialog in time): %v\n", err)
		}
		fmt.Printf("  result: %s\n", status)
		if status == permissions.StatusDenied {
			fmt.Println("  denied — cannot re-prompt; the only next step is OpenSystemSettings (see -open)")
		}
	}

	if *openKind != "" {
		k := permissions.Kind(*openKind)
		fmt.Printf("\nopening System Settings for %s...\n", k)
		if err := svc.OpenSystemSettings(k); err != nil {
			return fmt.Errorf("open system settings for %s: %w", k, err)
		}
	}

	return nil
}

func printStatus(svc *permissions.Service, k permissions.Kind) {
	status := svc.Check(k)
	fmt.Printf("  %-16s %s\n", k, status)
	switch status {
	case permissions.StatusNotDetermined:
		fmt.Printf("    rationale: %s\n", rationaleFor(svc, k))
	case permissions.StatusDenied:
		fmt.Printf("    guidance:  %s\n", deniedFor(svc, k))
	}
}

func rationaleFor(svc *permissions.Service, k permissions.Kind) string {
	switch k {
	case permissions.Notifications:
		return svc.Text(permissions.RationaleNotifications)
	case permissions.Accessibility:
		return svc.Text(permissions.RationaleAccessibility)
	case permissions.FullDiskAccess:
		return svc.Text(permissions.RationaleFullDiskAccess)
	default:
		return svc.Text(permissions.Unsupported)
	}
}

func deniedFor(svc *permissions.Service, k permissions.Kind) string {
	switch k {
	case permissions.Notifications:
		return svc.Text(permissions.DeniedNotifications)
	case permissions.Accessibility:
		return svc.Text(permissions.DeniedAccessibility)
	case permissions.FullDiskAccess:
		return svc.Text(permissions.DeniedFullDiskAccess)
	default:
		return svc.Text(permissions.Unsupported)
	}
}
