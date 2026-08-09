package settings

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/jrschumacher/wails-kit/v2/keyring"
)

// SecretMask is the sentinel value returned for password fields that have a value.
// When SetValues receives this value for a password field, it treats it as a no-op.
const SecretMask = "••••••••"

type Service struct {
	schema   Schema
	store    *Store
	secrets  keyring.Store
	onChange []func(values map[string]any)
	mu       sync.Mutex

	// appName and storagePath are staged by ServiceOptions and resolved into
	// s.store once every option has run — see NewService. Resolving eagerly
	// inside the option funcs (the previous design) made WithAppName and
	// WithStoragePath order-dependent: whichever ran last silently won,
	// discarding the other. Options must compose regardless of order.
	appName     string
	storagePath string
}

type ServiceOption func(*Service)

// WithAppName sets the app name for the settings store path. Combines with
// any other ServiceOption regardless of the order they're passed to
// NewService — see WithStoragePath.
func WithAppName(name string) ServiceOption {
	return func(s *Service) { s.appName = name }
}

// WithStoragePath overrides the default OS-standard settings file path.
// Use this for workspace-local configs where settings live alongside
// project files (e.g., git-tracked workspace directories).
//
// If both WithAppName and WithStoragePath are passed, WithStoragePath wins
// — regardless of which was passed first. Password fields are still stored
// in the OS keyring and never written to this path.
//
//	svc := settings.NewService(
//	    settings.WithStoragePath(filepath.Join(workspaceDir, "config.json")),
//	)
func WithStoragePath(path string) ServiceOption {
	return func(s *Service) { s.storagePath = path }
}

// WithStorePath overrides the settings file path.
// Deprecated: Use WithStoragePath instead.
func WithStorePath(path string) ServiceOption {
	return WithStoragePath(path)
}

// WithKeyring sets the keyring store for secret/password fields.
//
// If omitted, secrets default to an in-memory store: they work for the
// life of the process but are lost on restart. NewService logs a warning
// (slog.Warn) when this happens, because losing a user's saved API keys on
// every relaunch is a bad-enough surprise that it must never be silent —
// pass a persistent keyring.Store (e.g. keyring.NewEnvelopeStore or
// keyring.NewOSStore) in any app that isn't a short-lived test.
func WithKeyring(store keyring.Store) ServiceOption {
	return func(s *Service) {
		s.secrets = store
	}
}

// WithGroup adds a settings group to the schema.
func WithGroup(g Group) ServiceOption {
	return func(s *Service) {
		s.schema.Groups = append(s.schema.Groups, g)
	}
}

// WithOnChange registers a callback invoked after successful SetValues.
// Callbacks are invoked after the service's internal lock has been
// released, so a callback that calls back into the Service (e.g. GetValues
// or SetValues) never deadlocks.
func WithOnChange(fn func(values map[string]any)) ServiceOption {
	return func(s *Service) {
		s.onChange = append(s.onChange, fn)
	}
}

// NewService creates a new settings service.
func NewService(opts ...ServiceOption) *Service {
	s := &Service{appName: "app"}
	for _, opt := range opts {
		opt(s)
	}

	if s.storagePath != "" {
		s.store = NewStore(s.appName, WithPath(s.storagePath))
	} else {
		s.store = NewStore(s.appName)
	}

	if s.secrets == nil {
		slog.Warn("settings: no keyring configured — secret/password fields are stored in memory and will be lost on restart; pass settings.WithKeyring(...) to persist them")
		s.secrets = keyring.NewMemoryStore()
	}

	// Register defaults and known keys from schema fields
	defaults := make(map[string]any)
	knownKeys := make(map[string]bool)
	for _, group := range s.schema.Groups {
		for _, field := range group.Fields {
			knownKeys[field.Key] = true
			if field.Default != nil && field.Type != FieldPassword {
				defaults[field.Key] = field.Default
			}
		}
	}
	s.store.SetDefaults(defaults)
	s.store.SetKnownKeys(knownKeys)

	return s
}

// GetSchema returns the settings schema.
func (s *Service) GetSchema() Schema {
	return s.schema
}

// GetValues returns all current settings values.
// Password fields are masked — they return SecretMask if set, "" if not.
func (s *Service) GetValues() (map[string]any, error) {
	values, err := s.store.Load()
	if err != nil {
		return values, err
	}
	s.maskSecrets(values)

	// Run compute functions after secrets are masked so a compute func never
	// sees a raw secret either.
	for _, group := range s.schema.Groups {
		for key, fn := range group.ComputeFuncs {
			values[key] = fn(values)
		}
	}

	return values, nil
}

// maskSecrets overlays password fields onto values with their masked
// representation (SecretMask if set in the keyring, "" if not).
func (s *Service) maskSecrets(values map[string]any) {
	for _, group := range s.schema.Groups {
		for _, field := range group.Fields {
			if field.Type != FieldPassword {
				continue
			}
			if s.secrets.Has(field.Key) {
				values[field.Key] = SecretMask
			} else {
				values[field.Key] = ""
			}
		}
	}
}

// GetSecret retrieves the actual (unmasked) value of a password field.
// This is for internal/backend Go use only. It must never be reachable
// from the frontend — register Service.Binding() with Wails, never the
// Service itself, or every exported method (including this one) becomes
// callable from webview JS.
func (s *Service) GetSecret(key string) (string, error) {
	return s.secrets.Get(key)
}

// effectiveValues merges the persisted/default state with submitted, so
// validation runs against what the settings will actually look like after
// the write — not just the raw partial payload. Without this, omitting a
// condition's controlling field from a partial update makes conditionMet
// see "" and treat the field as hidden, skipping its validation entirely
// even though the field's real (persisted) controlling value would have
// required it. Must be called with s.mu held, since it reads s.secrets and
// s.store.
func (s *Service) effectiveValues(submitted map[string]any) (map[string]any, error) {
	current, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	s.maskSecrets(current)

	merged := make(map[string]any, len(current)+len(submitted))
	for k, v := range current {
		merged[k] = v
	}
	for k, v := range submitted {
		merged[k] = v
	}
	return merged, nil
}

// SetValues validates and saves settings. Password fields with the mask
// sentinel are skipped (no change). Empty string clears a secret. A
// non-string value for a password field is rejected as a validation error
// rather than being coerced to "" and deleting the stored secret.
//
// Validation runs against the *effective* state — defaults and persisted
// values overlaid with this submission — not the raw submitted payload, so
// a partial update can't sneak past a condition or a required field by
// simply omitting the controlling key.
//
// This method is safe for concurrent use. The internal lock is released
// before onChange callbacks run, so a callback that calls back into the
// Service does not deadlock.
func (s *Service) SetValues(values map[string]any) ([]ValidationError, error) {
	s.mu.Lock()

	merged, err := s.effectiveValues(values)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}

	if errs := Validate(s.schema, merged); errs != nil {
		s.mu.Unlock()
		return errs, nil
	}

	// Separate secrets from regular values. Only keys present in the
	// submission are persisted/updated — effectiveValues was only for
	// validation.
	toSave := make(map[string]any)
	computed := s.computedKeys()
	passwords := s.passwordKeys()

	for k, v := range values {
		if computed[k] {
			continue
		}
		if passwords[k] {
			str, ok := v.(string)
			if !ok {
				// Validation above guarantees every submitted password
				// value is a string; this is a defensive backstop so a
				// future bypass of Validate can never fall through to
				// treating a non-string as "" and deleting the secret.
				s.mu.Unlock()
				return nil, fmt.Errorf("settings: non-string value for password field %q slipped past validation", k)
			}
			if str == SecretMask {
				continue // No change
			}
			if str == "" {
				if err := s.secrets.Delete(k); err != nil {
					s.mu.Unlock()
					return nil, err
				}
			} else {
				if err := s.secrets.Set(k, str); err != nil {
					s.mu.Unlock()
					return nil, err
				}
			}
			continue
		}
		toSave[k] = v
	}

	if err := s.store.Save(toSave); err != nil {
		s.mu.Unlock()
		return nil, err
	}

	// Build the onChange snapshot with the lock still held (it reads
	// s.secrets), then release the lock before invoking any callback. A
	// callback that calls back into the Service (GetValues, SetValues, ...)
	// would otherwise deadlock against this same mutex.
	notifyValues := make(map[string]any, len(toSave)+len(passwords))
	for k, v := range toSave {
		notifyValues[k] = v
	}
	for k := range passwords {
		if s.secrets.Has(k) {
			notifyValues[k] = SecretMask
		}
	}
	callbacks := make([]func(map[string]any), len(s.onChange))
	copy(callbacks, s.onChange)

	s.mu.Unlock()

	for _, fn := range callbacks {
		fn(notifyValues)
	}

	return nil, nil
}

func (s *Service) computedKeys() map[string]bool {
	keys := make(map[string]bool)
	for _, group := range s.schema.Groups {
		for _, field := range group.Fields {
			if field.Type == FieldComputed {
				keys[field.Key] = true
			}
		}
	}
	return keys
}

func (s *Service) passwordKeys() map[string]bool {
	keys := make(map[string]bool)
	for _, group := range s.schema.Groups {
		for _, field := range group.Fields {
			if field.Type == FieldPassword {
				keys[field.Key] = true
			}
		}
	}
	return keys
}
