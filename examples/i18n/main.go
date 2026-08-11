// Command i18n-example demonstrates the i18n package headlessly: building a
// Localizer with an app-supplied catalog on top of the kit's own, resolving
// plain and pluralized strings, changing locale at runtime, and hydrating a
// catalog map the way a Wails frontend would via i18n.Binding.
//
// It uses an in-repo embedded catalog and never touches the OS locale, an
// env var, or a settings file — safe to run repeatedly and safe to run in
// CI.
package main

import (
	"embed"
	"fmt"
	"log"

	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/i18n"
)

//go:embed locales/*.json
var appCatalog embed.FS

// Text values are declared once, at the call site, pairing a stable catalog
// key with the English fallback. This mirrors how any app would declare its
// own strings — see locales/en.json and locales/fr.json for the catalog
// entries these resolve against.
var (
	greeting = i18n.T("myapp.example.greeting", "Hello, %s!")
	fileCnt  = i18n.T("myapp.example.file_count", "%d file(s)")
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	mem := events.NewMemoryEmitter()

	// WithCatalog(appCatalog) merges after the kit's own embedded catalog,
	// so app entries win on key collision (AD-5). WithLocale pins the
	// locale explicitly here so the example is deterministic regardless of
	// the machine running it; a real app typically omits it and lets the
	// settings/env/OS tiers decide (see i18n/AGENTS.md's resolution order).
	l, err := i18n.New(
		i18n.WithCatalog(appCatalog),
		i18n.WithLocale("en"),
		i18n.WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		return fmt.Errorf("new localizer: %w", err)
	}

	fmt.Println("locale:", l.Locale())
	fmt.Println(l.T(greeting, "Ryan"))
	fmt.Println(l.TN(fileCnt, 1))
	fmt.Println(l.TN(fileCnt, 5))

	// Missing keys fall back to the Text's built-in English string — this
	// never panics or returns an empty result.
	unknown := i18n.T("myapp.example.does_not_exist", "fallback text")
	fmt.Println(l.T(unknown))

	fmt.Println("--- switching to fr ---")
	if err := l.SetLocale("fr"); err != nil {
		return fmt.Errorf("set locale: %w", err)
	}
	fmt.Println("locale:", l.Locale())
	fmt.Println(l.T(greeting, "Ryan"))
	fmt.Println(l.TN(fileCnt, 1))
	fmt.Println(l.TN(fileCnt, 5))
	fmt.Printf("i18n:changed events emitted: %d\n", mem.Count())

	// Binding is what gets registered with Wails — GetCatalog/GetLocale/
	// SetLocale only, the same frontend-safe-surface pattern settings uses.
	binding := l.Binding()
	catalog := binding.GetCatalog()
	fmt.Printf("hydrated catalog has %d keys for locale %q\n", len(catalog), binding.GetLocale())

	return nil
}
