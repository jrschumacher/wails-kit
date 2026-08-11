// Command settings-example demonstrates the settings package headlessly:
// defining a schema, reading/writing values, the masking behavior around
// password fields, and (WP-12) resolving Field/Group labels through an
// *i18n.Localizer — including what changes when there's no localizer at
// all, and what changes when the locale is switched at runtime.
//
// It uses a throwaway temp directory for the settings file and
// keyring.NewMemoryStore() for secrets, so this example never touches a
// real config directory or OS keychain — safe to run repeatedly and safe to
// run in CI.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing/fstest"

	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// themeLabel is a real catalog key (not example.* Text{Other: ...} zero-key
// literals like the other labels in this file) so the example can show it
// actually resolving through a locale catalog below.
var themeLabel = i18n.T("example.settings.theme.label", "Theme")

// frCatalog is a tiny embedded French catalog, just enough to demonstrate
// GetSchema() re-resolving after SetLocale. A real app would ship this as
// locales/fr.json via i18n.WithCatalog(someEmbedFS) instead of an in-memory
// fstest.MapFS.
var frCatalog = fstest.MapFS{
	"locales/fr.json": &fstest.MapFile{Data: []byte(`{"example.settings.theme.label": "Thème"}`)},
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "wails-kit-settings-example-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	secrets := keyring.NewMemoryStore()

	// WithLocalizer is optional (WP-12) — a Service built without one
	// resolves every i18n.Text to its literal Other value, so localization
	// is strictly opt-in. l starts on English; SetLocale below switches it.
	l, err := i18n.New(i18n.WithCatalog(frCatalog), i18n.WithLocale("en"))
	if err != nil {
		return fmt.Errorf("i18n.New: %w", err)
	}

	svc := settings.NewService(
		settings.WithStoragePath(filepath.Join(tmp, "settings.json")),
		settings.WithKeyring(secrets),
		settings.WithLocalizer(l),
		settings.WithGroup(appearanceGroup()),
		settings.WithGroup(llmGroup()),
		settings.WithOnChange(func(values map[string]any) {
			fmt.Println("onChange fired with:", values)
		}),
	)
	fmt.Println("settings file:", svc.GetSchema().Groups[0].Key, "...")

	// GetSchema resolves fresh every call — no cached snapshot — so
	// switching the localizer's locale is immediately visible on the next
	// call, with no need to re-register groups or rebuild the service.
	fmt.Println("theme label (en):", svc.GetSchema().Groups[0].Fields[0].Label)
	if err := l.SetLocale("fr"); err != nil {
		return fmt.Errorf("set locale: %w", err)
	}
	fmt.Println("theme label (fr):", svc.GetSchema().Groups[0].Fields[0].Label)
	if err := l.SetLocale("en"); err != nil {
		return fmt.Errorf("set locale: %w", err)
	}

	// A Service with no localizer configured at all still works — every
	// i18n.Text just renders as its literal Other value. Localization is
	// opt-in; a consumer that never touches i18n sees no behavior change.
	noLocalizerSvc := settings.NewService(settings.WithKeyring(secrets), settings.WithGroup(appearanceGroup()))
	fmt.Println("theme label (no localizer):", noLocalizerSvc.GetSchema().Groups[0].Fields[0].Label)

	// GetSchema is what a frontend renders from — every field, type,
	// options, and visibility condition needed to build a form generically.
	schema := svc.GetSchema()
	fmt.Printf("schema has %d groups\n", len(schema.Groups))

	// Before anything is set, defaults apply and the password field reads
	// back as "" (not present in the keyring yet).
	values, err := svc.GetValues()
	if err != nil {
		return fmt.Errorf("get values: %w", err)
	}
	fmt.Println("theme (default):", values["theme"])
	fmt.Println("api_key (unset):", fmt.Sprintf("%q", values["api_key"]))

	// SetValues validates against *effective* state (defaults + persisted +
	// this submission), not just the raw payload — so a partial update
	// can't sneak an invalid value past a condition by omitting the
	// controlling field. Here we set provider, theme, and a real API key in
	// one call.
	errs, err := svc.SetValues(map[string]any{
		"theme":    "dark",
		"provider": "anthropic",
		"api_key":  "sk-ant-example-not-a-real-key",
	})
	if err != nil {
		return fmt.Errorf("set values: %w", err)
	}
	if errs != nil {
		return fmt.Errorf("unexpected validation errors: %v", errs)
	}

	// GetValues never returns the raw secret — it comes back masked.
	values, err = svc.GetValues()
	if err != nil {
		return fmt.Errorf("get values: %w", err)
	}
	fmt.Println("theme (after set):", values["theme"])
	fmt.Println("api_key (masked):", values["api_key"])

	// GetSecret is the one method that returns the raw value. It exists
	// only on *Service — never on *Binding — precisely so that registering
	// the wrong type with Wails is a compile error, not a runtime leak.
	//
	//	app.RegisterService(application.NewService(svc.Binding())) // frontend-safe
	//	app.RegisterService(application.NewService(svc))           // DO NOT: leaks GetSecret
	secret, err := svc.GetSecret("api_key")
	if err != nil {
		return fmt.Errorf("get secret: %w", err)
	}
	fmt.Println("api_key (raw, backend-only):", secret)

	// Sending the mask sentinel back is a no-op — this is how a frontend
	// "leaves the password field alone" without ever seeing the real value.
	if _, err := svc.SetValues(map[string]any{"api_key": settings.SecretMask}); err != nil {
		return fmt.Errorf("set values (mask no-op): %w", err)
	}
	secretAfterMaskSubmit, err := svc.GetSecret("api_key")
	if err != nil {
		return fmt.Errorf("get secret: %w", err)
	}
	fmt.Println("api_key unchanged after mask resubmit:", secretAfterMaskSubmit == secret)

	// The Binding is what actually gets registered with Wails. Its method
	// set is exactly GetSchema/GetValues/SetValues.
	binding := svc.Binding()
	bindingValues, err := binding.GetValues()
	if err != nil {
		return fmt.Errorf("binding get values: %w", err)
	}
	fmt.Println("via Binding, api_key is still masked:", bindingValues["api_key"])

	return nil
}

func appearanceGroup() settings.Group {
	return settings.Group{
		Key:   "appearance",
		Label: i18n.Text{Other: "Appearance"},
		Fields: []settings.Field{
			{
				Key:     "theme",
				Type:    settings.FieldSelect,
				Label:   themeLabel,
				Default: "system",
				Options: []settings.SelectOption{
					{Label: i18n.Text{Other: "System"}, Value: "system"},
					{Label: i18n.Text{Other: "Light"}, Value: "light"},
					{Label: i18n.Text{Other: "Dark"}, Value: "dark"},
				},
			},
		},
	}
}

func llmGroup() settings.Group {
	return settings.Group{
		Key:   "llm",
		Label: i18n.Text{Other: "LLM Provider"},
		Fields: []settings.Field{
			{
				Key:     "provider",
				Type:    settings.FieldSelect,
				Label:   i18n.Text{Other: "Provider"},
				Default: "anthropic",
				Options: []settings.SelectOption{
					{Label: i18n.Text{Other: "Anthropic"}, Value: "anthropic"},
					{Label: i18n.Text{Other: "OpenAI"}, Value: "openai"},
				},
			},
			{
				Key:   "api_key",
				Type:  settings.FieldPassword,
				Label: i18n.Text{Other: "API Key"},
				// Required only while a provider is selected — demonstrates
				// that validation now sees the *effective* provider value
				// (persisted or submitted), not just what's in a given
				// partial payload.
				Validation: &settings.Validation{Required: true},
				Condition:  &settings.Condition{Field: "provider", Equals: []string{"anthropic", "openai"}},
			},
		},
	}
}
