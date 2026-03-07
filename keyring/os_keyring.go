package keyring

import (
	gokeyring "github.com/zalando/go-keyring"
)

// OSStore uses the OS keyring (macOS Keychain, Windows Credential Manager,
// Linux Secret Service) with optional environment variable fallback.
type OSStore struct {
	serviceName string
	envPrefix   string
}

// OSStoreOption configures an OSStore.
type OSStoreOption func(*OSStore)

// WithEnvPrefix sets the environment variable prefix for fallback lookups.
// For example, WithEnvPrefix("MYAPP") causes Get("api.key") to check
// MYAPP_API_KEY before querying the OS keyring.
func WithEnvPrefix(prefix string) OSStoreOption {
	return func(s *OSStore) { s.envPrefix = prefix }
}

// NewOSStore creates a keyring store backed by the OS credential manager.
func NewOSStore(serviceName string, opts ...OSStoreOption) *OSStore {
	s := &OSStore{serviceName: serviceName}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *OSStore) Set(key, value string) error {
	return gokeyring.Set(s.serviceName, key, value)
}

func (s *OSStore) Get(key string) (string, error) {
	val, err := gokeyring.Get(s.serviceName, key)
	if err != nil {
		if err == gokeyring.ErrNotFound {
			// Try env var fallback
			if v, ok := envFallback(s.envPrefix, key); ok {
				return v, nil
			}
			return "", ErrNotFound
		}
		// Other keyring errors — still try env fallback
		if v, ok := envFallback(s.envPrefix, key); ok {
			return v, nil
		}
		return "", err
	}
	return val, nil
}

func (s *OSStore) Delete(key string) error {
	err := gokeyring.Delete(s.serviceName, key)
	if err == gokeyring.ErrNotFound {
		return nil
	}
	return err
}

func (s *OSStore) Has(key string) bool {
	_, err := s.Get(key)
	return err == nil
}
