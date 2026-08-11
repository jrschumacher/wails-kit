// Command anyllm-example demonstrates settings/templates/anyllm headlessly:
// building an llmconfig settings group, setting a selection, and using
// BuildProvider to construct a real any-llm-go client.
//
// It uses a throwaway temp directory for the settings file and
// keyring.NewMemoryStore() for secrets, and the API key set below is not a
// real credential — constructing a provider never makes a network call, so
// this example is safe to run repeatedly and safe to run in CI.
//
// This example lives inside the settings/templates/anyllm nested module
// (it has its own go.mod one directory up) because it needs any-llm-go.
// examples/llmconfig in the main module is the SDK-free counterpart.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"
	"github.com/jrschumacher/wails-kit/v2/settings/templates/llmconfig"

	anyllm "github.com/jrschumacher/wails-kit/settings/templates/anyllm/v2"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "wails-kit-anyllm-example-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	group, cfg := llmconfig.New(
		llmconfig.WithProviders("anthropic", "openai"),
		llmconfig.WithDefaultProvider("anthropic"),
	)

	svc := settings.NewService(
		settings.WithStoragePath(filepath.Join(tmp, "settings.json")),
		settings.WithKeyring(keyring.NewMemoryStore()),
		settings.WithGroup(group),
	)

	if _, err := svc.SetValues(map[string]any{
		"llm.provider":         "anthropic",
		"llm.model":            "claude-sonnet-4-6",
		"llm.anthropic.secret": "sk-ant-example-not-a-real-key",
	}); err != nil {
		return fmt.Errorf("set values: %w", err)
	}

	// BuildProvider reads the selection through cfg (from llmconfig) and
	// constructs the corresponding any-llm-go client. No network call
	// happens here — that's deferred to provider.Completion/ListModels.
	provider, modelID, err := anyllm.BuildProvider(svc, cfg)
	if err != nil {
		return fmt.Errorf("build provider: %w", err)
	}
	fmt.Printf("built provider %q for model %q\n", provider.Name(), modelID)

	// ListModels demonstrates the "supported" and "unsupported" paths. For
	// Anthropic (selected above) it returns ErrModelListingUnsupported
	// without making any network call — that check happens before any
	// request is attempted.
	if _, err := anyllm.ListModels(context.Background(), provider); errors.Is(err, anyllm.ErrModelListingUnsupported) {
		fmt.Println("provider does not support live model listing; fall back to llmconfig.Builtin()")
	}

	// Switching provider and re-reading the selection shows BuildProvider
	// picking up the new choice with no other code changes.
	if _, err := svc.SetValues(map[string]any{
		"llm.provider":      "openai",
		"llm.model":         "gpt-4o",
		"llm.openai.secret": "sk-example-not-a-real-key",
	}); err != nil {
		return fmt.Errorf("set values (openai): %w", err)
	}
	provider, modelID, err = anyllm.BuildProvider(svc, cfg)
	if err != nil {
		return fmt.Errorf("build provider (openai): %w", err)
	}
	fmt.Printf("switched to provider %q for model %q\n", provider.Name(), modelID)

	return nil
}
