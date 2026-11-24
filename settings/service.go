package settings

import (
	"reflect"
	"sync"
	"time"

	"github.com/jrschumacher/wails-kit/events"
	"github.com/jrschumacher/wails-kit/keyring"
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

	emitter      *events.Emitter
	checkTimeout time.Duration
	// gen counts the changes to each key (guarded by mu): CheckValue bumps it
	// when it starts, SetValues when it changes the key. A check saves only if
	// its key's generation is still the one it started with.
	gen      map[string]uint64
	failures map[string]ValidationError // guarded by mu

	// The onChange notifications waiting to be delivered, queued under mu in
	// save order and delivered by deliver() without it (guarded by notifyMu).
	notifyMu    sync.Mutex
	notifyQueue []map[string]any
	delivering  bool
}

type ServiceOption func(*Service)

// WithAppName sets the app name for the settings store path.
func WithAppName(name string) ServiceOption {
	return func(s *Service) {
		s.store = NewStore(name)
	}
}

// WithStoragePath overrides the default OS-standard settings file path.
// Use this for workspace-local configs where settings live alongside
// project files (e.g., git-tracked workspace directories).
//
// When set, this takes precedence over WithAppName. Password fields are
// still stored in the OS keyring and never written to this path.
//
//	svc := settings.NewService(
//	    settings.WithStoragePath(filepath.Join(workspaceDir, "config.json")),
//	)
func WithStoragePath(path string) ServiceOption {
	return func(s *Service) {
		if s.store == nil {
			s.store = NewStore("app", WithPath(path))
		} else {
			s.store.path = path
		}
	}
}

// WithStorePath overrides the settings file path.
// Deprecated: Use WithStoragePath instead.
func WithStorePath(path string) ServiceOption {
	return WithStoragePath(path)
}

// WithKeyring sets the keyring store for secret/password fields.
// If not set, secrets are stored in a MemoryStore (not persisted).
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
func WithOnChange(fn func(values map[string]any)) ServiceOption {
	return func(s *Service) {
		s.onChange = append(s.onChange, fn)
	}
}

// NewService creates a new settings service.
func NewService(opts ...ServiceOption) *Service {
	s := &Service{
		checkTimeout: DefaultCheckTimeout,
		gen:          make(map[string]uint64),
		failures:     make(map[string]ValidationError),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.store == nil {
		s.store = NewStore("app")
	}
	if s.secrets == nil {
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

	// Load secrets (masked) and run compute functions
	for _, group := range s.schema.Groups {
		for _, field := range group.Fields {
			if field.Type == FieldPassword {
				if s.secrets.Has(field.Key) {
					values[field.Key] = SecretMask
				} else {
					values[field.Key] = ""
				}
			}
		}
		for key, fn := range group.ComputeFuncs {
			values[key] = fn(values)
		}
	}

	return values, nil
}

// GetSecret retrieves the actual (unmasked) value of a password field.
// This is for internal/backend use — never expose to the frontend.
func (s *Service) GetSecret(key string) (string, error) {
	return s.secrets.Get(key)
}

// SetValues validates and saves settings. Password fields with the mask
// sentinel are skipped (no change). Empty string clears a secret.
// A key whose value it changes has its running check made out of date and
// its recorded check failure dropped (see CheckValue). A key it is given
// with the value already saved (the mask, for a password) is not a change:
// it doesn't overtake a check running for that key, so the window's saves
// of other fields, which carry every value, leave its checks alone.
// The onChange callbacks run after the service's lock is released, in save
// order, so they may call back into the service.
// This method is safe for concurrent use.
func (s *Service) SetValues(values map[string]any) ([]ValidationError, error) {
	s.mu.Lock()
	errs, err := s.setValuesLocked(values)
	s.mu.Unlock()
	// The onChange callbacks run after the lock is released, so they may call
	// back into the service, and in the order the saves happened.
	s.deliver()
	return errs, err
}

func (s *Service) setValuesLocked(values map[string]any) ([]ValidationError, error) {
	if errs := Validate(s.schema, values); errs != nil {
		return errs, nil
	}

	// The keys this call changes: their running checks are out of date and
	// their recorded check failures no longer apply.
	current, _ := s.store.Load()
	for k, v := range values {
		if s.changes(k, v, current) {
			s.gen[k]++
			delete(s.failures, k)
		}
	}

	// Separate secrets from regular values
	toSave := make(map[string]any)
	computed := s.computedKeys()
	passwords := s.passwordKeys()

	for k, v := range values {
		if computed[k] {
			continue
		}
		if passwords[k] {
			str, _ := v.(string)
			if str == SecretMask {
				continue // No change
			}
			if str == "" {
				_ = s.secrets.Delete(k)
			} else {
				if err := s.secrets.Set(k, str); err != nil {
					return nil, err
				}
			}
			continue
		}
		toSave[k] = v
	}

	if err := s.store.Save(toSave); err != nil {
		return nil, err
	}

	// Notify listeners with the full value set (secrets masked)
	notifyValues := make(map[string]any)
	for k, v := range toSave {
		notifyValues[k] = v
	}
	for k := range passwords {
		if s.secrets.Has(k) {
			notifyValues[k] = SecretMask
		}
	}
	s.queueNotify(notifyValues)

	return nil, nil
}

// queueNotify queues one onChange notification. The caller holds s.mu, so
// notifications queue in the order the saves happened.
func (s *Service) queueNotify(values map[string]any) {
	if len(s.onChange) == 0 {
		return
	}
	s.notifyMu.Lock()
	s.notifyQueue = append(s.notifyQueue, values)
	s.notifyMu.Unlock()
}

// deliver runs the queued onChange notifications, one at a time and in
// order, without holding s.mu. If another call is already delivering (on
// another goroutine, or further up this one's stack when a callback saved
// again), it leaves the queue to that call, which delivers it next.
func (s *Service) deliver() {
	s.notifyMu.Lock()
	if s.delivering {
		s.notifyMu.Unlock()
		return
	}
	s.delivering = true
	defer func() {
		s.notifyMu.Lock()
		s.delivering = false
		s.notifyMu.Unlock()
	}()
	for len(s.notifyQueue) > 0 {
		values := s.notifyQueue[0]
		s.notifyQueue = s.notifyQueue[1:]
		s.notifyMu.Unlock()
		for _, fn := range s.onChange {
			fn(values)
		}
		s.notifyMu.Lock()
	}
	s.notifyMu.Unlock()
}

// changes reports whether SetValues writing v to key changes it: a new secret
// (not the mask), or a value other than the one saved.
func (s *Service) changes(key string, v any, current map[string]any) bool {
	if s.passwordKeys()[key] {
		str, _ := v.(string)
		if str == SecretMask {
			return false
		}
		if str == "" {
			return s.secrets.Has(key)
		}
		old, err := s.secrets.Get(key)
		return err != nil || old != str
	}
	if s.computedKeys()[key] {
		return false
	}
	return !reflect.DeepEqual(current[key], v)
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
