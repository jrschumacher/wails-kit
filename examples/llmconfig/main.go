// Command llmconfig-example demonstrates settings/templates/llmconfig headlessly:
// building the provider/model/API-key settings group, wiring it into a
// settings.Service, and reading the effective selection back out with
// Config.Selection/BaseURL.
//
// It uses a throwaway temp directory for the settings file and
// keyring.NewMemoryStore() for secrets, so this example never touches a real
// config directory or OS keychain — safe to run repeatedly and safe to run
// in CI.
//
// This package imports no LLM SDK — that's the point of the split described
// in settings/templates/llmconfig/README.md. To see the selection actually
// turned into a callable LLM client, see the separate nested module
// settings/templates/anyllm and its own example.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"
	"github.com/jrschumacher/wails-kit/v2/settings/templates/llmconfig"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "wails-kit-llmconfig-example-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// WithModels demonstrates the fix for the kit's historical defect: the
	// shipped model list is a default, not a hardcoded ceiling. Here we add
	// a model to the built-in Anthropic list without forking the package.
	group, cfg := llmconfig.New(
		llmconfig.WithProviders("anthropic", "openai"),
		llmconfig.WithDefaultProvider("anthropic"),
		llmconfig.WithModels("anthropic", append(anthropicBuiltinModels(),
			settings.SelectOption{Label: "Claude Next (example)", Value: "claude-next-example"},
		)),
	)

	svc := settings.NewService(
		settings.WithStoragePath(filepath.Join(tmp, "settings.json")),
		settings.WithKeyring(keyring.NewMemoryStore()),
		settings.WithGroup(group),
	)

	// GetSchema is what a frontend renders from — provider/model selects,
	// the per-provider advanced fields, and the added "Claude Next" model.
	schema := svc.GetSchema()
	fmt.Printf("schema has %d group(s), %d field(s) in %q\n",
		len(schema.Groups), len(schema.Groups[0].Fields), schema.Groups[0].Key)

	// Before anything is set, Selection reports the configured default
	// provider with no model chosen and no API key.
	providerID, modelID, apiKey, err := cfg.Selection(svc)
	if err != nil {
		return fmt.Errorf("selection (before set): %w", err)
	}
	fmt.Printf("before SetValues: provider=%q model=%q apiKey=%q\n", providerID, modelID, apiKey)

	errs, err := svc.SetValues(map[string]any{
		"llm.provider":         "anthropic",
		"llm.model":            "claude-next-example",
		"llm.anthropic.secret": "sk-ant-example-not-a-real-key",
	})
	if err != nil {
		return fmt.Errorf("set values: %w", err)
	}
	if errs != nil {
		return fmt.Errorf("unexpected validation errors: %v", errs)
	}

	providerID, modelID, apiKey, err = cfg.Selection(svc)
	if err != nil {
		return fmt.Errorf("selection (after set): %w", err)
	}
	fmt.Printf("after SetValues: provider=%q model=%q apiKey=%q\n", providerID, modelID, apiKey)

	// A per-provider base-URL override, useful for self-hosted or proxied
	// endpoints. Unset here, so it reads back empty.
	baseURL, err := cfg.BaseURL(svc, providerID)
	if err != nil {
		return fmt.Errorf("base url: %w", err)
	}
	fmt.Printf("base URL override: %q (empty means use the provider default)\n", baseURL)

	// A custom-model override takes precedence over the selected model —
	// this is what lets a user point at a fine-tune or preview snapshot the
	// built-in catalog doesn't know about, without needing a code change.
	if _, err := svc.SetValues(map[string]any{"llm.anthropic.customModel": "my-fine-tune-id"}); err != nil {
		return fmt.Errorf("set custom model: %w", err)
	}
	_, modelID, _, err = cfg.Selection(svc)
	if err != nil {
		return fmt.Errorf("selection (custom model): %w", err)
	}
	fmt.Printf("resolved model after custom override: %q\n", modelID)

	return nil
}

func anthropicBuiltinModels() []settings.SelectOption {
	for _, p := range llmconfig.Builtin() {
		if p.ID == "anthropic" {
			return p.Models
		}
	}
	return nil
}
