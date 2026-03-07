package keyring

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrNotFound is returned when a key does not exist in the store.
var ErrNotFound = errors.New("keyring: key not found")

// Store is the interface for credential storage backends.
type Store interface {
	Set(key, value string) error
	Get(key string) (string, error)
	Delete(key string) error
	Has(key string) bool
}

// SetJSON marshals value as JSON and stores it.
func SetJSON(s Store, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("keyring: marshal %s: %w", key, err)
	}
	return s.Set(key, string(data))
}

// GetJSON retrieves a value and unmarshals it into target.
func GetJSON(s Store, key string, target any) error {
	data, err := s.Get(key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(data), target); err != nil {
		return fmt.Errorf("keyring: unmarshal %s: %w", key, err)
	}
	return nil
}

// EnvKey converts a field key to an environment variable name.
// Example: envPrefix="MYAPP", key="llm.anthropic.secret" -> "MYAPP_LLM_ANTHROPIC_SECRET"
func EnvKey(envPrefix, key string) string {
	k := strings.ToUpper(key)
	k = strings.ReplaceAll(k, ".", "_")
	k = strings.ReplaceAll(k, "-", "_")
	return envPrefix + "_" + k
}

// envFallback checks the environment variable for a key.
func envFallback(envPrefix, key string) (string, bool) {
	if envPrefix == "" {
		return "", false
	}
	v := os.Getenv(EnvKey(envPrefix, key))
	if v == "" {
		return "", false
	}
	return v, true
}
