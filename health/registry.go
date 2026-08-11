package health

import (
	"context"
	"sync"
	"time"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
)

// DefaultCheckName is the name Registry auto-registers its built-in
// connectivity check under (see New / WithConnectivityProbe). Remove it at
// runtime with Unregister(DefaultCheckName) if an app wants no default
// connectivity check at all.
const DefaultCheckName = "connectivity"

// defaultInterval is used when neither a Check's own Interval nor
// WithDefaultInterval is set.
const defaultInterval = 60 * time.Second

// entry is a registered check plus its mutable last-known status and the
// machinery to stop its background probe goroutine independently of the
// Registry-wide context (Unregister while Start is running).
type entry struct {
	check  Check
	status CheckStatus
	stopCh chan struct{}
}

// Registry probes registered Checks on a cadence (or on demand), caches
// their last-known CheckStatus, and exposes a resolved Snapshot plus
// health:changed events on state transitions. Build one with New; it is
// safe for concurrent use.
type Registry struct {
	mu     sync.Mutex
	checks map[string]*entry
	order  []string // registration order, for deterministic Snapshot output

	emitter           *events.Emitter
	defaultInterval   time.Duration
	connectivityProbe Probe
	skipDefaultCheck  bool

	running bool
	runCtx  context.Context //nolint:containedctx // scheduling loop needs it for Register-while-running
	wg      sync.WaitGroup
}

// Option configures a Registry. See New.
type Option func(*Registry)

// WithEmitter wires an events.Emitter so EventChanged fires on state
// transitions. Nil (the default) means transitions update state silently.
func WithEmitter(e *events.Emitter) Option {
	return func(r *Registry) { r.emitter = e }
}

// WithDefaultInterval sets the probing cadence used by any Check that
// doesn't set its own Interval. Default 60s.
func WithDefaultInterval(d time.Duration) Option {
	return func(r *Registry) {
		if d > 0 {
			r.defaultInterval = d
		}
	}
}

// WithConnectivityProbe overrides the Probe used by the registry's built-in
// connectivity check (see DefaultCheckName). Default: an HTTP probe
// expecting 204 from a well-known captive-portal-check URL — see
// defaultConnectivityProbe in httpprobe.go. Tests and apps with an offline
// notion of "online" (e.g. probing a LAN gateway) should always override
// this; the default performs a real network request.
func WithConnectivityProbe(p Probe) Option {
	return func(r *Registry) { r.connectivityProbe = p }
}

// WithoutDefaultConnectivityCheck skips auto-registering the built-in
// connectivity check entirely. Equivalent to calling
// Unregister(DefaultCheckName) immediately after New, but avoids the brief
// window where the check exists (and could be raced by a concurrent
// Snapshot/Start caller).
func WithoutDefaultConnectivityCheck() Option {
	return func(r *Registry) { r.skipDefaultCheck = true }
}

// New builds a Registry and auto-registers its built-in connectivity check
// (see DefaultCheckName, WithConnectivityProbe, WithoutDefaultConnectivityCheck).
func New(opts ...Option) *Registry {
	r := &Registry{
		checks:          make(map[string]*entry),
		defaultInterval: defaultInterval,
	}
	for _, opt := range opts {
		opt(r)
	}

	if !r.skipDefaultCheck {
		probe := r.connectivityProbe
		if probe == nil {
			probe = defaultConnectivityProbe()
		}
		// Registration cannot fail here: the map is empty and the name and
		// probe are always valid. Ignoring the error is deliberate — New
		// has no error return, matching every other kit constructor that
		// takes only Options.
		_, _ = r.Register(Check{
			Name:     DefaultCheckName,
			Class:    ClassConnectivity,
			Critical: true,
			Probe:    probe,
		})
	}

	return r
}

// Register adds c to the registry. It returns a remove func that
// unregisters c (safe to call more than once; a no-op after the first
// call), or an error if c is invalid or c.Name is already registered.
//
// If the registry is currently running (Start was called and its context
// is not yet Done), the new check's probe loop starts immediately.
func (r *Registry) Register(c Check) (remove func(), err error) {
	if c.Name == "" {
		return nil, errors.Newf(ErrInvalidCheck, "health: check name is required")
	}
	if c.Probe == nil {
		return nil, errors.Newf(ErrInvalidCheck, "health: check %q: probe is required", c.Name)
	}

	r.mu.Lock()
	if _, exists := r.checks[c.Name]; exists {
		r.mu.Unlock()
		return nil, errors.Newf(ErrDuplicateCheck, "health: check %q is already registered", c.Name)
	}

	e := &entry{
		check: c,
		status: CheckStatus{
			Name:     c.Name,
			Class:    c.Class,
			State:    StateUnknown,
			Critical: c.Critical,
		},
		stopCh: make(chan struct{}),
	}
	r.checks[c.Name] = e
	r.order = append(r.order, c.Name)

	running := r.running
	runCtx := r.runCtx
	if running {
		r.wg.Add(1)
		go r.runCheck(runCtx, c.Name, e.stopCh)
	}
	r.mu.Unlock()

	var once sync.Once
	return func() { once.Do(func() { r.Unregister(c.Name) }) }, nil
}

// Unregister removes the named check, stopping its background probe loop
// if the registry is running. Returns false if no check by that name was
// registered.
func (r *Registry) Unregister(name string) bool {
	r.mu.Lock()
	e, ok := r.checks[name]
	if !ok {
		r.mu.Unlock()
		return false
	}
	delete(r.checks, name)
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	r.mu.Unlock()

	// Closing stopCh is safe whether or not a goroutine is reading it: if
	// the registry isn't running, nothing was ever started to receive the
	// signal, and the channel is simply garbage-collected with e.
	close(e.stopCh)
	return true
}

// Snapshot returns the registry's current resolved view. See Snapshot's
// field docs for the offline-suppression and Overall-computation rules.
func (r *Registry) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

func (r *Registry) snapshotLocked() Snapshot {
	offline := false
	for _, name := range r.order {
		e := r.checks[name]
		if e.check.Class == ClassConnectivity && e.status.State == StateDown {
			offline = true
			break
		}
	}

	checks := make([]CheckStatus, 0, len(r.order))
	overall := StateUnknown
	haveCritical := false

	for _, name := range r.order {
		e := r.checks[name]
		st := e.status

		if offline && e.check.Class != ClassConnectivity && st.State != StateUnknown {
			// "Your API is down" must never be claimed when the real
			// problem is "there is no network at all" — suppress to
			// unknown rather than down. The underlying state is not lost:
			// it already drove health:changed when it transitioned, and
			// resumes reporting normally once connectivity returns.
			st = CheckStatus{
				Name:      st.Name,
				Class:     st.Class,
				State:     StateUnknown,
				Critical:  st.Critical,
				CheckedAt: st.CheckedAt,
			}
		}

		checks = append(checks, st)

		if st.Critical {
			if !haveCritical {
				overall = st.State
				haveCritical = true
			} else {
				overall = worseState(overall, st.State)
			}
		}
	}

	// No critical checks (including an empty registry) means nothing is
	// vouching for overall health — report unknown, not healthy.
	if !haveCritical {
		overall = StateUnknown
	}

	return Snapshot{Overall: overall, Offline: offline, Checks: checks}
}
