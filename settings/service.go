package settings

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sync"
)

type Service struct {
	schema Schema
	store  *Store
	// appName and storePath are collected from the options and only turned
	// into a Store once, in NewService, so that option order cannot matter.
	appName   string
	storePath string
	// patterns holds every Validation.Pattern in the schema, compiled and
	// anchored once at construction rather than on each SetValues call.
	patterns map[string]*regexp.Regexp
	// knownKeys, computedKeys and secretKeys are derived from the schema at
	// construction; the schema is immutable once NewService returns.
	knownKeys    map[string]bool
	computedKeys map[string]bool
	secretKeys   map[string]bool
	// secrets holds every FieldPassword value. secretsPath is collected from
	// WithPlaintextFileSecrets and, like storePath, only resolved in NewService
	// so option order cannot matter.
	secrets     SecretStore
	secretsPath string
	onChange    []func(values map[string]any)
	mu          sync.Mutex
}

type ServiceOption func(*Service)

// WithAppName sets the application name used to derive the default config
// path. It is ignored if WithStorePath is also supplied, in either order.
func WithAppName(name string) ServiceOption {
	return func(s *Service) {
		s.appName = name
	}
}

// WithStorePath sets an explicit config file path, overriding the path derived
// from WithAppName regardless of the order the two options are given in.
func WithStorePath(path string) ServiceOption {
	return func(s *Service) {
		s.storePath = path
	}
}

func WithGroup(g Group) ServiceOption {
	return func(s *Service) {
		s.schema.Groups = append(s.schema.Groups, g)
	}
}

func WithOnChange(fn func(values map[string]any)) ServiceOption {
	return func(s *Service) {
		s.onChange = append(s.onChange, fn)
	}
}

// WithSecretStore replaces the backend used for FieldPassword values.
//
// The default is the OS keyring. Supply this to inject a MemorySecretStore in
// tests, or a backend of your own. It takes precedence over
// WithPlaintextFileSecrets regardless of the order the two are given in.
func WithSecretStore(store SecretStore) ServiceOption {
	return func(s *Service) {
		s.secrets = store
	}
}

// WithPlaintextFileSecrets opts out of the OS keyring and stores secrets in a
// plaintext JSON file at path, mode 0600.
//
// This is never selected automatically. The keyring is unreachable in some real
// environments — headless Linux with no D-Bus or libsecret, some containers —
// and there the honest options are to fail loudly or to have the operator
// choose this explicitly. Falling back silently is not among them: it would
// leave users believing their API keys are in the Keychain while they sit
// readable on disk and inside every backup.
func WithPlaintextFileSecrets(path string) ServiceOption {
	return func(s *Service) {
		s.secretsPath = path
	}
}

// NewService builds a settings service from the supplied options.
//
// It returns an error rather than panicking when the environment or the schema
// is unusable: the user config directory cannot be resolved, or ValidateSchema
// rejects the schema. Both are fatal for the caller and both are things an
// application wants to report at startup.
//
// Construction is also where a corrupt config file is detected and quarantined.
// That is not an error — the service is fully usable afterwards — so check
// Service.Corruption once NewService returns and tell the user if it is
// non-nil.
func NewService(opts ...ServiceOption) (*Service, error) {
	s := &Service{}
	for _, opt := range opts {
		opt(s)
	}

	// Build the store once, after every option has run: constructing it inside
	// the options made them order-dependent, with a later WithAppName silently
	// discarding an earlier WithStorePath.
	appName := s.appName
	if appName == "" {
		appName = "app"
	}
	var storeOpts []StoreOption
	if s.storePath != "" {
		storeOpts = append(storeOpts, WithPath(s.storePath))
	}
	store, err := NewStore(appName, storeOpts...)
	if err != nil {
		return nil, err
	}
	s.store = store

	// Probe the config file once so that a corrupt one is quarantined here,
	// during startup, rather than at some later moment the app cannot predict.
	// After this returns, Corruption() is authoritative for what the user needs
	// to be told. See CorruptConfig.
	s.store.quarantineIfCorrupt()

	s.knownKeys, s.computedKeys, s.secretKeys = schemaKeys(s.schema)

	// Register defaults from schema fields. Password fields are excluded: their
	// value lives in the SecretStore, and a default would otherwise surface
	// through GetValues indistinguishably from a real stored secret.
	defaults := make(map[string]any)
	for _, group := range s.schema.Groups {
		for _, field := range group.Fields {
			if field.Default != nil && !s.secretKeys[field.Key] {
				defaults[field.Key] = field.Default
			}
		}
	}
	s.store.SetDefaults(defaults)

	// Check the schema itself before anything can use it. A pattern that does
	// not compile, a Condition on a field that does not exist, a select with no
	// options — all are developer errors, and all must surface here, at
	// construction, rather than reaching an end user as "invalid format" or as
	// a control that never appears. See ValidateSchema for the full list.
	if err := ValidateSchema(s.schema); err != nil {
		return nil, err
	}

	// Compile once rather than on every SetValues. ValidateSchema has already
	// established that every pattern compiles.
	s.patterns, err = compilePatterns(s.schema)
	if err != nil {
		return nil, err
	}

	// Secrets default to the OS keyring, and only an explicit option moves them
	// off it. An injected store wins over a plaintext file path so the two
	// options are commutative, matching WithStorePath / WithAppName.
	if s.secrets == nil {
		if s.secretsPath != "" {
			s.secrets = NewPlaintextFileSecretStore(s.secretsPath)
		} else {
			s.secrets = NewKeyringSecretStore(appName)
		}
	}

	return s, nil
}

func (s *Service) GetSchema() Schema {
	return s.schema
}

// Corruption reports whether the config file was found to be unparseable and
// had to be moved aside, and returns nil otherwise.
//
// A settings file containing invalid JSON — a half-written file from a machine
// that lost power, a hand edit gone wrong, a disk that dropped a block — cannot
// be recovered by retrying. Rather than failing every save from then on and
// leaving a non-technical user with an app that will not remember anything and
// a file they cannot find, the damaged file is renamed to
// "<path>.corrupt-<timestamp>" and the service continues on schema defaults.
//
// That is a destructive-looking recovery, so it is never silent. Check this
// immediately after NewService, which probes the file for exactly this reason,
// and tell the user:
//
//	svc, err := settings.NewService(settings.WithAppName("myapp"), ...)
//	if err != nil {
//		return err
//	}
//	if c := svc.Corruption(); c != nil {
//		ui.Warn(fmt.Sprintf(
//			"Your settings could not be read and have been reset to defaults.\n"+
//				"The previous file was saved as %s", c.QuarantinePath))
//	}
//
// It stays accurate for the life of the service: a file corrupted while the app
// is running is quarantined on the next read, and this reports that one. The
// returned pointer is a copy and is safe to retain.
//
// Only malformed JSON is quarantined. A file that cannot be read at all —
// permission denied, an I/O error — is still returned as an error from
// GetValues and SetValues, because there the user's settings are intact and
// destroying them would be the wrong answer.
func (s *Service) Corruption() *CorruptConfig {
	return s.store.Corruption()
}

// GetValues returns the current settings for display in the UI.
//
// Password fields never carry their real value here. This is the map that
// crosses the Wails bridge into the webview on every settings render, so a raw
// API key in it is one XSS or one hostile frontend dependency away from being
// exfiltrated. Instead, a password field whose secret is stored is reported as
// SecretSentinel, and one with no stored secret is omitted entirely.
//
// Use GetValuesWithSecrets or GetSecret from Go code that needs the real value.
func (s *Service) GetValues() (map[string]any, error) {
	return s.values(s.maskSecret)
}

// GetValuesWithSecrets returns the current settings with real password values
// resolved from the SecretStore.
//
// This is the Go-side counterpart to GetValues, for backend code that must
// actually use a secret — constructing an LLM provider from a stored API key,
// for example. The result MUST NOT be returned to the frontend or passed to
// anything that logs it. Password fields with no stored secret are omitted.
func (s *Service) GetValuesWithSecrets() (map[string]any, error) {
	return s.values(s.resolveSecret)
}

// values loads the persisted settings, applies resolve to every password field,
// then runs compute functions.
//
// Compute functions run last and therefore observe whatever resolve produced.
// That ordering matters for GetValues: were it reversed, a compute function
// reading a password key would copy the real secret into a non-password field
// and carry it across the bridge in spite of the masking.
func (s *Service) values(resolve func(key string, values map[string]any) error) (map[string]any, error) {
	// Take the same lock SetValues writes under, so a read concurrent with a
	// save returns one coherent state rather than a mix of the two — secrets
	// are written before the config file, so without this a reader can observe
	// a new API key alongside the old provider that does not go with it.
	//
	// onChange listeners are invoked after this lock is released, so a listener
	// calling GetValues does not deadlock.
	s.mu.Lock()
	defer s.mu.Unlock()

	values, err := s.store.Load()
	if err != nil {
		return values, err
	}

	for key := range s.secretKeys {
		// Drop anything the file may hold for a password key before resolving.
		// A config file written by an older build can contain a plaintext
		// secret, and it must not be served just because it is there.
		delete(values, key)
		if err := resolve(key, values); err != nil {
			return nil, err
		}
	}

	// Run compute functions
	for _, group := range s.schema.Groups {
		for key, fn := range group.ComputeFuncs {
			values[key] = fn(values)
		}
	}

	return values, nil
}

// maskSecret sets key to SecretSentinel when a secret is stored, and leaves the
// key absent otherwise.
func (s *Service) maskSecret(key string, values map[string]any) error {
	if _, err := s.secrets.Get(key); err != nil {
		if errors.Is(err, ErrSecretNotFound) {
			return nil
		}
		// A backend that cannot be read is reported, not treated as "unset".
		// Silently rendering an empty field would tell the user their key was
		// never saved, and invite them to overwrite one that is fine.
		return err
	}
	values[key] = SecretSentinel
	return nil
}

// resolveSecret sets key to the real stored secret, leaving it absent when
// nothing is stored.
func (s *Service) resolveSecret(key string, values map[string]any) error {
	v, err := s.secrets.Get(key)
	if err != nil {
		if errors.Is(err, ErrSecretNotFound) {
			return nil
		}
		return err
	}
	values[key] = v
	return nil
}

// GetSecret returns the real stored value of a password field.
//
// This is the security boundary: real secrets are reachable from Go and never
// from the frontend. Keep it off any struct handed to Wails for binding — see
// Bindings.
//
// key must be a FieldPassword declared in the schema. The error wraps
// ErrSecretNotFound when the user has not set a value.
func (s *Service) GetSecret(key string) (string, error) {
	if err := s.requireSecretField(key); err != nil {
		return "", err
	}
	return s.secrets.Get(key)
}

// SetSecret stores a password field's value directly, without going through
// SetValues. Useful for secrets the user never types, such as an OAuth token
// obtained by the backend.
func (s *Service) SetSecret(key, value string) error {
	if err := s.requireSecretField(key); err != nil {
		return err
	}
	return s.secrets.Set(key, value)
}

// DeleteSecret removes a password field's stored value. Deleting a value that
// is not there is not an error.
func (s *Service) DeleteSecret(key string) error {
	if err := s.requireSecretField(key); err != nil {
		return err
	}
	return s.secrets.Delete(key)
}

func (s *Service) requireSecretField(key string) error {
	if !s.knownKeys[key] {
		return fmt.Errorf("settings: %q is not a known setting", key)
	}
	if !s.secretKeys[key] {
		return fmt.Errorf("settings: %q is not a password field; use GetValues or SetValues", key)
	}
	return nil
}

// SetValues validates values and merges them into the persisted settings.
//
// Only keys declared in the schema may be set: an undeclared key is reported as
// a ValidationError and the whole call is rejected without writing anything.
// Rejecting rather than silently dropping is deliberate — a silent drop is
// indistinguishable from a successful save, which is how the frontend could
// previously write arbitrary JSON into the config file and never find out.
//
// values may be partial — one settings group from the frontend, say. Keys it
// omits keep whatever is already persisted; it never erases them.
//
// Validation runs against the state that will be in effect after the save —
// schema defaults, then the config file, then values, with secrets resolved and
// compute functions applied — not against the submitted map alone. On a partial
// submission the difference is the whole ballgame: judged on the raw map, every
// Required field in every group the user did not touch looks empty, and every
// Condition whose controlling field was not submitted looks unmet, so saving
// one group failed on fields the user had already filled in while genuinely
// visible fields escaped their rules entirely.
//
// Nothing is written unless validation passes completely: not the config file,
// and not the SecretStore.
//
// Values for computed fields are accepted (the frontend round-trips them) but
// are never persisted, and are recomputed before validation rather than taken
// on trust.
//
// Password fields are routed to the SecretStore instead of the config file:
//
//   - SecretSentinel means "leave the stored secret alone". A settings page
//     renders the sentinel and posts it back untouched, so editing an unrelated
//     field cannot clobber a stored API key.
//   - An empty string clears the stored secret.
//   - Any other value replaces it.
//
// Secrets are written before the config file. A failure to reach the secret
// backend therefore aborts the whole call with nothing persisted, rather than
// saving the ordinary settings and reporting an error the user is likely to
// read as "nothing was saved".
//
// A non-nil []ValidationError means the user must fix something; a non-nil
// error means the save could not be attempted. Exactly one of the two is ever
// non-nil.
func (s *Service) SetValues(values map[string]any) ([]ValidationError, error) {
	pre := append(s.unknownKeyErrors(values), s.secretTypeErrors(values)...)

	listeners, changed, errs, err := s.persist(values, pre)
	if err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return errs, nil
	}

	// Notify listeners with no lock held. A callback that calls back into
	// GetValues or SetValues is entirely reasonable for a settings framework;
	// invoking it under s.mu would deadlock.
	for _, fn := range listeners {
		fn(changed)
	}

	return nil, nil
}

// persist runs the load-validate-merge-save cycle under s.mu and returns
// snapshots of the change listeners and of the submitted values, both taken
// under that same lock. The cycle is a read-modify-write over two separate
// store-lock acquisitions, so without s.mu two concurrent SetValues calls
// interleave between the read and the write and one update is silently lost.
//
// errs carries any problems found before the lock was taken (undeclared keys);
// validation errors are appended to it, so a caller sees every reason its
// submission was rejected in one round trip.
func (s *Service) persist(values map[string]any, errs []ValidationError) ([]func(map[string]any), map[string]any, []ValidationError, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// One snapshot of the persisted state drives both validation and the merge,
	// so the two cannot disagree about what is already saved.
	persisted, err := s.store.loadPersisted()
	if err != nil {
		return nil, nil, nil, err
	}

	// Validate before touching the secret backend or the config file, so an
	// invalid submission is a pure no-op.
	effective, err := s.effectiveValues(persisted, values)
	if err != nil {
		return nil, nil, nil, err
	}
	errs = append(errs, validate(s.schema, effective, s.patterns)...)
	if len(errs) > 0 {
		return nil, nil, errs, nil
	}

	// Secrets next. If the backend is unreachable this fails before the config
	// file has been touched, leaving the previous state completely intact.
	if err := s.applySecrets(values); err != nil {
		return nil, nil, nil, err
	}

	// Merge onto what is already persisted: a caller sending a partial map
	// (e.g. one settings group from the frontend) must not erase unrelated
	// keys, which for a secret with no default would be a silent total loss.
	toSave := persisted
	for k, v := range values {
		// Strip computed fields before persisting
		if !s.computedKeys[k] && !s.secretKeys[k] {
			toSave[k] = v
		}
	}
	// A password key must never be in the config file, including one left there
	// by an older build that stored secrets in plaintext.
	for k := range s.secretKeys {
		delete(toSave, k)
	}

	if err := s.store.Save(toSave); err != nil {
		return nil, nil, nil, err
	}

	return slices.Clone(s.onChange), s.maskedClone(values), nil, nil
}

// effectiveValues builds the value set that will be in effect once submitted is
// saved on top of persisted: schema defaults, then the config file, then the
// submission, with secrets resolved and compute functions applied.
//
// Validation runs against this rather than against the raw submission. A
// frontend that posts one settings group at a time sends a partial map, and
// judging a Condition — or any cross-field rule — on a partial map gets it
// wrong in both directions: a field whose controlling key was not submitted
// looks hidden and escapes validation entirely, while a Required field in an
// untouched group looks empty and blocks the save.
func (s *Service) effectiveValues(persisted, submitted map[string]any) (map[string]any, error) {
	effective := s.store.defaultsClone()
	maps.Copy(effective, persisted)
	maps.Copy(effective, submitted)

	for key := range s.secretKeys {
		// Never trust the config file for a password key: an older build may
		// have left a plaintext value there.
		delete(effective, key)

		v, isStr := submitted[key].(string)
		if isStr && v != SecretSentinel {
			// The value about to be stored. An empty string clears the secret,
			// so it must leave the key absent — that is what makes a Required
			// password field reject a submission that clears it.
			if v != "" {
				effective[key] = v
			}
			continue
		}
		// Not submitted, or submitted as the sentinel: whatever is stored stays
		// in effect. Resolving it here is what stops a client from satisfying a
		// Required password field by echoing the placeholder back when no
		// secret exists.
		if err := s.resolveSecret(key, effective); err != nil {
			return nil, err
		}
	}

	// Computed values are derived, so recompute rather than trusting whatever
	// the frontend round-tripped. They run last, over the resolved secrets,
	// exactly as in values().
	for _, group := range s.schema.Groups {
		for key, fn := range group.ComputeFuncs {
			effective[key] = fn(effective)
		}
	}

	return effective, nil
}

// applySecrets writes every submitted password value to the SecretStore. The
// sentinel is skipped, since it means the stored value is to be left alone, and
// an empty submission clears the stored value.
func (s *Service) applySecrets(values map[string]any) error {
	for key := range s.secretKeys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		v, isStr := raw.(string)
		if !isStr || v == SecretSentinel {
			continue
		}
		if v == "" {
			if err := s.secrets.Delete(key); err != nil {
				return err
			}
			continue
		}
		if err := s.secrets.Set(key, v); err != nil {
			return err
		}
	}
	return nil
}

// maskedClone is the payload handed to onChange listeners. Listeners are
// arbitrary application code that may log or forward what they receive, so the
// raw secret is replaced by the sentinel there too; a listener that needs the
// real value should call GetSecret.
func (s *Service) maskedClone(values map[string]any) map[string]any {
	changed := maps.Clone(values)
	for key := range s.secretKeys {
		v, ok := changed[key]
		if !ok {
			continue
		}
		if str, isStr := v.(string); isStr && str == "" {
			delete(changed, key)
			continue
		}
		changed[key] = SecretSentinel
	}
	return changed
}

// schemaKeys walks the schema once and returns the set of every declared field
// key, the subset that is computed and therefore never persisted, and the
// subset that is a password and therefore lives in the SecretStore.
func schemaKeys(schema Schema) (known, computed, secret map[string]bool) {
	known = make(map[string]bool)
	computed = make(map[string]bool)
	secret = make(map[string]bool)
	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			known[field.Key] = true
			switch field.Type {
			case FieldComputed:
				computed[field.Key] = true
			case FieldPassword:
				secret[field.Key] = true
			}
		}
	}
	return known, computed, secret
}

// secretTypeErrors reports every password field submitted as something other
// than a string.
//
// applySecrets can only route a string to the SecretStore, and the config file
// never holds a password key, so such a value used to vanish silently and the
// call still reported success — the user is told their API key was saved when
// nothing was written anywhere. That is the same failure the undeclared-key
// rejection exists to prevent, and it gets the same answer.
func (s *Service) secretTypeErrors(values map[string]any) []ValidationError {
	var keys []string
	for key := range s.secretKeys {
		v, ok := values[key]
		if !ok {
			continue
		}
		if _, isStr := v.(string); !isStr {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	slices.Sort(keys)

	errs := make([]ValidationError, 0, len(keys))
	for _, key := range keys {
		errs = append(errs, ValidationError{
			Field:   key,
			Message: fmt.Sprintf("%q is a password field and must be set to a string", key),
		})
	}
	return errs
}

// unknownKeyErrors reports every key in values that the schema does not
// declare, sorted so the result is deterministic.
func (s *Service) unknownKeyErrors(values map[string]any) []ValidationError {
	var unknown []string
	for k := range values {
		if !s.knownKeys[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)

	errs := make([]ValidationError, 0, len(unknown))
	for _, k := range unknown {
		errs = append(errs, ValidationError{
			Field:   k,
			Message: fmt.Sprintf("%q is not a known setting", k),
		})
	}
	return errs
}

// Bindings is the frontend-facing surface of a Service: exactly GetSchema,
// GetValues, SetValues and Corruption.
//
// Wails binds every exported method of a bound struct, so binding the Service
// itself would publish GetSecret and GetValuesWithSecrets to the webview and
// undo the entire point of routing secrets through a SecretStore. Bind this
// instead:
//
//	svc, err := settings.NewService(...)
//	// ...
//	app := application.New(application.Options{
//		Services: []application.Service{
//			application.NewService(svc.Bindings()),
//		},
//	})
//
// and keep the Service itself in Go for GetSecret and GetValuesWithSecrets.
type Bindings struct {
	svc *Service
}

// Bindings returns the value to hand to Wails for binding. See Bindings.
func (s *Service) Bindings() *Bindings {
	return &Bindings{svc: s}
}

func (b *Bindings) GetSchema() Schema {
	return b.svc.GetSchema()
}

// GetValues returns the masked value set; password fields carry SecretSentinel
// or are absent. See Service.GetValues.
func (b *Bindings) GetValues() (map[string]any, error) {
	return b.svc.GetValues()
}

func (b *Bindings) SetValues(values map[string]any) ([]ValidationError, error) {
	return b.svc.SetValues(values)
}

// Corruption returns the quarantine report, or null, so a settings page can
// render the "your settings were reset" notice itself. It carries no secret
// material — only file paths and a decoding error. See Service.Corruption.
func (b *Bindings) Corruption() *CorruptConfig {
	return b.svc.Corruption()
}
