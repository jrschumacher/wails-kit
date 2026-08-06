package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/jrschumacher/wails-kit/llm"
	"github.com/jrschumacher/wails-kit/settings"
)

// AppService is the application's own frontend-facing API.
//
// Every exported method of a bound type is published to the webview, so the
// rule that applies to settings.Service applies here too: nothing on this type
// may return a secret. It holds a *settings.Service and a *llm.ProviderManager,
// both of which can reach real API keys, but neither is reachable from JS
// because the fields are unexported and no method hands them out.
type AppService struct {
	app       *application.App
	settings  *settings.Service
	providers *llm.ProviderManager
	prefs     *UpdatePrefs
}

// CheckForUpdates is the "Check Now" button.
//
// The settings schema has no button field type, so an action like this is the
// application's own UI element calling the updater directly. CheckAndInstall
// opens the framework's default update window and runs the whole flow: check,
// download, verify, stage. The user presses "Restart & Apply" in that window,
// which is what triggers the binary swap.
func (a *AppService) CheckForUpdates() error {
	if a.app.Updater.State() == updater.StateUnconfigured {
		return errors.New("updates are not configured in this build")
	}
	a.prefs.MarkChecked(time.Now())
	return a.app.Updater.CheckAndInstall(context.Background())
}

// UpdaterState reports the updater's lifecycle phase for display. Safe to
// expose: it is a fixed vocabulary of strings, no paths and no user data.
func (a *AppService) UpdaterState() string {
	return string(a.app.Updater.State())
}

// LLMStatus reports which provider and model the current settings resolve to.
//
// This is the safe shape of "show me my LLM config": the manager resolves the
// real API key internally to build the client and never hands it back, and the
// two strings returned here carry no secret material.
func (a *AppService) LLMStatus() (string, error) {
	p, err := a.providers.Provider()
	if err != nil {
		// Typically "no API key set yet" surfacing as an auth error later, or a
		// provider name with no registered factory (a missing blank import).
		return "", fmt.Errorf("llm: %w", err)
	}
	return fmt.Sprintf("%s / %s", p.ProviderName(), p.ModelID()), nil
}
