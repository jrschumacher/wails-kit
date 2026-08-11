package llmconfig

import (
	"errors"
	"fmt"

	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// Config reads the effective LLM selection back out of a settings.Service.
// It holds nothing but the group key New built the schema under, so it is
// cheap to keep around and safe to call repeatedly — every method re-reads
// current state, it caches nothing.
type Config struct {
	groupKey string
}

// GroupKey returns the settings group key this Config reads from (the value
// passed to WithGroupKey, or "llm" by default).
func (c *Config) GroupKey() string {
	return c.groupKey
}

// Selection reads the currently selected provider ID, the effective model ID
// (a per-provider custom-model override takes precedence over the picked
// model), and the provider's API key from svc.
//
// providerID is "" when nothing has been selected yet (a fresh install with
// no default, or a group whose Schema was never given any providers) — that
// is not an error. err is non-nil only when the keyring read itself fails
// for a reason other than "no secret stored".
func (c *Config) Selection(svc *settings.Service) (providerID, modelID, apiKey string, err error) {
	values, err := svc.GetValues()
	if err != nil {
		return "", "", "", fmt.Errorf("llmconfig: read settings: %w", err)
	}

	providerID, _ = values[c.groupKey+".provider"].(string)
	modelID = resolveModelID(c.groupKey, values)
	if providerID == "" {
		return "", modelID, "", nil
	}

	apiKey, err = svc.GetSecret(c.groupKey + "." + providerID + ".secret")
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return providerID, modelID, "", nil
		}
		return providerID, modelID, "", fmt.Errorf("llmconfig: read API key: %w", err)
	}
	return providerID, modelID, apiKey, nil
}

// BaseURL reads the base-URL override for the given provider, "" if unset.
// Pass the providerID returned by Selection.
func (c *Config) BaseURL(svc *settings.Service, providerID string) (string, error) {
	if providerID == "" {
		return "", nil
	}
	values, err := svc.GetValues()
	if err != nil {
		return "", fmt.Errorf("llmconfig: read settings: %w", err)
	}
	baseURL, _ := values[c.groupKey+"."+providerID+".baseURL"].(string)
	return baseURL, nil
}
