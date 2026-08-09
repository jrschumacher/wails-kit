// Command errors-example demonstrates the errors package's i18n integration
// (WP-11): constructing errors before any localizer exists, registering a
// custom app error code with a translatable message, then installing a
// localizer and watching GetUserMessage (and JSON marshaling) resolve live
// against it — without reconstructing anything.
//
// It uses an in-repo embedded catalog and never touches the OS locale, an
// env var, or a settings file — safe to run repeatedly and safe to run in
// CI.
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"

	kiterrors "github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/i18n"
)

//go:embed locales/*.json
var appCatalog embed.FS

// syncConflict is a custom app error code, registered like any other kit
// package registers its own — a stable catalog key ("myapp.", never the
// kit-reserved "wailskit." prefix) paired with an English fallback.
const syncConflict kiterrors.Code = "sync_conflict"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	kiterrors.RegisterMessages(map[kiterrors.Code]i18n.Text{
		syncConflict: i18n.T("myapp.errors.sync_conflict", "A sync conflict occurred. Please resolve it manually."),
	})

	// Errors are frequently constructed before an app has finished wiring
	// its dependencies (e.g. in a package init()) — that's fine, because
	// resolution happens later, at read time, not here.
	notFound := kiterrors.New(kiterrors.ErrNotFound, "row 42 missing from table users", nil)
	conflict := kiterrors.New(syncConflict, "local rev 3 vs remote rev 5", nil)

	fmt.Println("--- no localizer installed ---")
	fmt.Println("not_found:", kiterrors.GetUserMessage(notFound))
	fmt.Println("sync_conflict:", kiterrors.GetUserMessage(conflict))
	fmt.Println("code is stable:", kiterrors.GetCode(conflict) == syncConflict)

	l, err := i18n.New(i18n.WithCatalog(appCatalog), i18n.WithLocale("fr"))
	if err != nil {
		return fmt.Errorf("new localizer: %w", err)
	}
	kiterrors.SetLocalizer(l)

	fmt.Println("--- localizer installed, locale fr ---")
	// Same *UserError values as above — nothing was reconstructed.
	fmt.Println("not_found:", kiterrors.GetUserMessage(notFound))
	fmt.Println("sync_conflict:", kiterrors.GetUserMessage(conflict))

	// The construction-time UserMsg field is an English snapshot and does
	// not change; GetUserMessage (and JSON marshaling) is what resolves
	// live. See errors/README.md's Localization section.
	fmt.Println("UserMsg field (still English):", notFound.UserMsg)

	data, err := json.Marshal(notFound)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	fmt.Println("JSON (userMsg resolved live):", string(data))

	fmt.Println("--- switching to en ---")
	if err := l.SetLocale("en"); err != nil {
		return fmt.Errorf("set locale: %w", err)
	}
	fmt.Println("not_found:", kiterrors.GetUserMessage(notFound))
	fmt.Println("sync_conflict:", kiterrors.GetUserMessage(conflict))

	return nil
}
