// Package lifecycle provides ordered startup and shutdown of services with
// dependency tracking. It topologically sorts services based on DependsOn
// declarations, starts them in dependency order, and shuts them down in
// reverse order. If a service fails to start, already-started services are
// rolled back.
package lifecycle

import (
	"context"
	stderrors "errors"
	"fmt"
	"sync"
	"time"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/i18n"
)

// Error codes for lifecycle operations.
const (
	ErrCyclicDependency errors.Code = "lifecycle_cyclic_dependency"
	ErrMissingDep       errors.Code = "lifecycle_missing_dependency"
	ErrStartup          errors.Code = "lifecycle_startup"
	ErrShutdown         errors.Code = "lifecycle_shutdown"
	ErrTimeout          errors.Code = "lifecycle_timeout"
	ErrDuplicateService errors.Code = "lifecycle_duplicate_service"
	ErrInvalidState     errors.Code = "lifecycle_invalid_state"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrCyclicDependency: i18n.T("wailskit.lifecycle.errors.cyclic_dependency", "Service configuration error: circular dependency detected."),
		ErrMissingDep:       i18n.T("wailskit.lifecycle.errors.missing_dependency", "Service configuration error: a required dependency is missing."),
		ErrStartup:          i18n.T("wailskit.lifecycle.errors.startup", "Failed to start a required service. Please try restarting the application."),
		ErrShutdown:         i18n.T("wailskit.lifecycle.errors.shutdown", "An error occurred while shutting down. Some resources may not have been cleaned up."),
		ErrTimeout:          i18n.T("wailskit.lifecycle.errors.timeout", "A service took too long to respond. Please try restarting the application."),
		ErrDuplicateService: i18n.T("wailskit.lifecycle.errors.duplicate_service", "Service configuration error: a service name is registered more than once."),
		ErrInvalidState:     i18n.T("wailskit.lifecycle.errors.invalid_state", "The application's service manager is busy or already running. Please try again."),
	})
}

// Event names emitted by the lifecycle manager.
const (
	EventStarted  = "lifecycle:started"
	EventStopped  = "lifecycle:stopped"
	EventError    = "lifecycle:error"
	EventRollback = "lifecycle:rollback"
	EventTimeout  = "lifecycle:timeout"
)

// ServiceStartedPayload is emitted when a service starts successfully.
type ServiceStartedPayload struct {
	Name string `json:"name"`
}

// ServiceStoppedPayload is emitted when a service stops.
type ServiceStoppedPayload struct {
	Name string `json:"name"`
}

// ErrorPayload is emitted when a service fails to start or stop.
type ErrorPayload struct {
	Name    string      `json:"name"`
	Message string      `json:"message"`
	Code    errors.Code `json:"code"`
}

// RollbackPayload is emitted when a partial startup failure triggers rollback.
type RollbackPayload struct {
	FailedService  string   `json:"failedService"`
	RollingBack    []string `json:"rollingBack"`
	RollbackErrors []string `json:"rollbackErrors,omitempty"`
}

// HealthStatus represents the health state of a service.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

// TimeoutPayload is emitted when a service operation times out.
type TimeoutPayload struct {
	Name    string `json:"name"`
	Phase   string `json:"phase"` // "startup" or "shutdown"
	Timeout string `json:"timeout"`
}

// ServiceHealth reports the health of a single service.
type ServiceHealth struct {
	Name   string       `json:"name"`
	Status HealthStatus `json:"status"`
}

// Service is the interface that managed services must implement.
type Service interface {
	OnStartup(ctx context.Context) error
	OnShutdown() error
}

// HealthChecker is an optional interface that services can implement
// to report their health status.
type HealthChecker interface {
	Health() HealthStatus
}

// entry holds a registered service and its dependency metadata.
type entry struct {
	name    string
	service Service
	deps    []string
	timeout time.Duration // per-service timeout override; 0 means use global
}

// managerState tracks what a Manager is currently doing, so Startup and
// Shutdown can detect and reject concurrent or repeated calls instead of
// silently corrupting m.started (see Startup/Shutdown for the guard).
type managerState int

const (
	stateIdle managerState = iota
	stateStarting
	stateStarted
	stateStopping
)

func (s managerState) String() string {
	switch s {
	case stateIdle:
		return "idle"
	case stateStarting:
		return "starting"
	case stateStarted:
		return "started"
	case stateStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

// Manager manages ordered startup and shutdown of services.
//
// entries and order are fixed after NewManager returns and are read without
// a lock. mu guards everything that changes across the lifetime of the
// Manager: state and started. Callers must not use a Manager concurrently
// beyond that guarantee — e.g. Health() during a Startup/Shutdown call is
// safe (it will see a consistent, if possibly stale, snapshot), but nothing
// makes it meaningful to call Startup from two goroutines "at the same
// time" beyond one of them deterministically losing the race and getting
// ErrInvalidState.
type Manager struct {
	entries []*entry
	order   []string // topologically sorted service names

	mu      sync.Mutex
	state   managerState
	started []string // services that have been started (in start order)

	emitter *events.Emitter
	timeout time.Duration // global timeout; 0 means no timeout
}

// ManagerOption configures a Manager.
type ManagerOption func(*Manager)

// ServiceOption configures an individual service entry.
type ServiceOption func(*entry)

// DependsOn declares that a service depends on the named services,
// which must be started first.
func DependsOn(names ...string) ServiceOption {
	return func(e *entry) {
		e.deps = append(e.deps, names...)
	}
}

// WithServiceTimeout sets a per-service timeout that overrides the global timeout.
func WithServiceTimeout(d time.Duration) ServiceOption {
	return func(e *entry) {
		e.timeout = d
	}
}

// WithService registers a named service with optional configuration.
func WithService(name string, svc Service, opts ...ServiceOption) ManagerOption {
	return func(m *Manager) {
		e := &entry{name: name, service: svc}
		for _, opt := range opts {
			opt(e)
		}
		m.entries = append(m.entries, e)
	}
}

// WithEmitter sets the event emitter for lifecycle events.
func WithEmitter(emitter *events.Emitter) ManagerOption {
	return func(m *Manager) {
		m.emitter = emitter
	}
}

// WithTimeout sets a global timeout for service startup and shutdown operations.
// Individual services can override this with WithServiceTimeout.
func WithTimeout(d time.Duration) ManagerOption {
	return func(m *Manager) {
		m.timeout = d
	}
}

// NewManager creates a Manager with the given options. It validates that all
// dependencies exist and performs a topological sort. Returns an error if
// there are missing dependencies or cycles.
func NewManager(opts ...ManagerOption) (*Manager, error) {
	m := &Manager{}
	for _, opt := range opts {
		opt(m)
	}

	order, err := m.topoSort()
	if err != nil {
		return nil, err
	}
	m.order = order
	return m, nil
}

// Order returns the resolved startup order of service names.
func (m *Manager) Order() []string {
	out := make([]string, len(m.order))
	copy(out, m.order)
	return out
}

// Startup starts all services in dependency order. If a service fails,
// already-started services are shut down in reverse order. The original
// startup error is always returned; rollback errors are joined.
//
// Startup returns ErrInvalidState instead of running if the Manager is
// already starting, already started, or shutting down — calling Startup a
// second time without an intervening Shutdown used to silently re-run
// OnStartup on every service (double-starting already-running ones) while
// discarding the previous m.started list, orphaning whatever it tracked.
func (m *Manager) Startup(ctx context.Context) error {
	m.mu.Lock()
	if m.state != stateIdle {
		state := m.state
		m.mu.Unlock()
		return errors.New(ErrInvalidState,
			fmt.Sprintf("Startup called while manager state is %v; call Shutdown before starting again", state), nil)
	}
	m.state = stateStarting
	m.started = nil
	m.mu.Unlock()

	byName := m.entryMap()

	for _, name := range m.order {
		e := byName[name]
		if err := m.startService(ctx, e); err != nil {
			startErr := errors.Wrap(ErrStartup, fmt.Sprintf("service %q failed to start", name), err).
				WithField("service", name)

			m.emit(EventError, ErrorPayload{
				Name:    name,
				Message: errors.GetUserMessage(startErr),
				Code:    ErrStartup,
			})

			// Rollback already-started services in reverse order.
			rollbackErrs := m.rollback(name)

			m.mu.Lock()
			m.state = stateIdle
			m.mu.Unlock()

			if rollbackErrs != nil {
				return stderrors.Join(startErr, rollbackErrs)
			}
			return startErr
		}

		m.mu.Lock()
		m.started = append(m.started, name)
		m.mu.Unlock()
		m.emit(EventStarted, ServiceStartedPayload{Name: name})
	}

	m.mu.Lock()
	m.state = stateStarted
	m.mu.Unlock()

	return nil
}

// Shutdown stops all started services in reverse startup order.
// It does not stop on the first error; all errors are collected and joined.
//
// Shutdown is a safe no-op if the Manager was never successfully started
// (state != stateStarted) — including a second concurrent call while a
// Shutdown is already in flight — rather than acting on a partial or
// already-cleared m.started list. This makes `defer mgr.Shutdown()`
// unconditionally safe regardless of whether Startup succeeded.
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	if m.state != stateStarted {
		m.mu.Unlock()
		return nil
	}
	m.state = stateStopping
	started := make([]string, len(m.started))
	copy(started, m.started)
	m.mu.Unlock()

	byName := m.entryMap()
	var errs []error

	// Shut down in reverse startup order.
	for i := len(started) - 1; i >= 0; i-- {
		name := started[i]
		e := byName[name]
		if err := m.stopService(e); err != nil {
			shutErr := errors.Wrap(ErrShutdown, fmt.Sprintf("service %q failed to shut down", name), err).
				WithField("service", name)

			m.emit(EventError, ErrorPayload{
				Name:    name,
				Message: errors.GetUserMessage(shutErr),
				Code:    ErrShutdown,
			})

			errs = append(errs, shutErr)
		} else {
			m.emit(EventStopped, ServiceStoppedPayload{Name: name})
		}
	}

	m.mu.Lock()
	m.started = nil
	m.state = stateIdle
	m.mu.Unlock()

	if len(errs) > 0 {
		return stderrors.Join(errs...)
	}
	return nil
}

// Health returns the health status of all started services. Services that
// implement HealthChecker report their own status; others are reported as healthy.
func (m *Manager) Health() []ServiceHealth {
	m.mu.Lock()
	started := make([]string, len(m.started))
	copy(started, m.started)
	m.mu.Unlock()

	byName := m.entryMap()
	result := make([]ServiceHealth, 0, len(started))

	for _, name := range started {
		e := byName[name]
		status := StatusHealthy
		if hc, ok := e.service.(HealthChecker); ok {
			status = hc.Health()
		}
		result = append(result, ServiceHealth{Name: name, Status: status})
	}

	return result
}

// serviceTimeout returns the effective timeout for a service entry.
func (m *Manager) serviceTimeout(e *entry) time.Duration {
	if e.timeout > 0 {
		return e.timeout
	}
	return m.timeout
}

// startService starts a single service, applying a timeout if configured.
func (m *Manager) startService(ctx context.Context, e *entry) error {
	timeout := m.serviceTimeout(e)
	if timeout <= 0 {
		return e.service.OnStartup(ctx)
	}

	startCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- e.service.OnStartup(startCtx) }()

	select {
	case err := <-done:
		return err
	case <-startCtx.Done():
		if ctx.Err() != nil {
			// Parent context was cancelled, not a timeout.
			return ctx.Err()
		}
		m.emit(EventTimeout, TimeoutPayload{
			Name:    e.name,
			Phase:   "startup",
			Timeout: timeout.String(),
		})
		// The manager is about to report this service as failed and move
		// on to rollback (or return the timeout error to the Startup
		// caller), but the goroutine above is still running OnStartup(e)
		// against a service that may ignore context cancellation. If it
		// eventually succeeds, it will never be in m.started — nothing
		// will ever call its OnShutdown — and it leaks, still running,
		// for the life of the process. Reap it in the background: wait
		// for the call to finish and, if it did succeed after all, shut
		// it straight back down.
		go m.reapTimedOutStartup(e, done)
		return errors.New(ErrTimeout,
			fmt.Sprintf("service %q startup timed out after %s", e.name, timeout), startCtx.Err()).
			WithField("service", e.name).
			WithField("timeout", timeout.String())
	}
}

// reapTimedOutStartup waits for a service's OnStartup call to finish after
// startService has already reported it as timed out and the manager has
// moved on. See the call site in startService. Best-effort: there is no
// in-flight Startup/Shutdown caller left to return an error to, so a
// resulting OnShutdown failure is only reported via EventError.
func (m *Manager) reapTimedOutStartup(e *entry, done <-chan error) {
	if err := <-done; err == nil {
		if shutErr := e.service.OnShutdown(); shutErr != nil {
			wrapped := errors.Wrap(ErrShutdown,
				fmt.Sprintf("service %q started after its timeout had already been reported; shutdown of the orphaned instance also failed", e.name),
				shutErr).WithField("service", e.name)
			m.emit(EventError, ErrorPayload{
				Name:    e.name,
				Message: errors.GetUserMessage(wrapped),
				Code:    ErrShutdown,
			})
		}
	}
}

// stopService stops a single service, applying a timeout if configured.
func (m *Manager) stopService(e *entry) error {
	timeout := m.serviceTimeout(e)
	if timeout <= 0 {
		return e.service.OnShutdown()
	}

	done := make(chan error, 1)
	go func() { done <- e.service.OnShutdown() }()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		m.emit(EventTimeout, TimeoutPayload{
			Name:    e.name,
			Phase:   "shutdown",
			Timeout: timeout.String(),
		})
		return errors.New(ErrTimeout,
			fmt.Sprintf("service %q shutdown timed out after %s", e.name, timeout), nil).
			WithField("service", e.name).
			WithField("timeout", timeout.String())
	}
}

// rollback shuts down already-started services in reverse order after a
// startup failure. Returns joined errors or nil.
//
// It shuts each service down through stopService — the same path Shutdown
// uses — rather than calling OnShutdown directly, so WithTimeout /
// WithServiceTimeout apply here too. A service that hangs in OnShutdown
// during rollback used to block Startup forever despite a configured
// timeout existing for exactly this situation.
func (m *Manager) rollback(failedName string) error {
	m.mu.Lock()
	rollingBack := make([]string, len(m.started))
	copy(rollingBack, m.started)
	m.mu.Unlock()

	byName := m.entryMap()

	// Reverse for display.
	for i, j := 0, len(rollingBack)-1; i < j; i, j = i+1, j-1 {
		rollingBack[i], rollingBack[j] = rollingBack[j], rollingBack[i]
	}

	var errs []error
	var errMsgs []string

	for _, name := range rollingBack {
		e := byName[name]
		if err := m.stopService(e); err != nil {
			errs = append(errs, errors.Wrap(ErrShutdown, fmt.Sprintf("rollback: service %q failed to shut down", name), err))
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", name, err))
		}
	}

	m.emit(EventRollback, RollbackPayload{
		FailedService:  failedName,
		RollingBack:    rollingBack,
		RollbackErrors: errMsgs,
	})

	m.mu.Lock()
	m.started = nil
	m.mu.Unlock()

	if len(errs) > 0 {
		return stderrors.Join(errs...)
	}
	return nil
}

// topoSort performs a topological sort using Kahn's algorithm.
// Returns an error on duplicate names, missing dependencies, or cycles.
func (m *Manager) topoSort() ([]string, error) {
	names := make(map[string]bool, len(m.entries))
	for _, e := range m.entries {
		// Reject duplicate names before anything else. Without this check,
		// this same map collapses two entries into one key, so the
		// len(order) != len(m.entries) cycle-detection guard below never
		// fires (order comes out the same length as the deduped name set,
		// which coincidentally still differs from len(m.entries) — but not
		// reliably, and the real damage happens earlier: entryMap() is
		// last-writer-wins, so name resolution during Startup silently
		// starts the *other* entry's service, not the caller's expected
		// one, while the actual entry with that name is never started.
		if names[e.name] {
			return nil, errors.New(ErrDuplicateService,
				fmt.Sprintf("service %q is registered more than once", e.name), nil).
				WithField("service", e.name)
		}
		names[e.name] = true
	}

	// Validate all dependencies exist.
	for _, e := range m.entries {
		for _, dep := range e.deps {
			if !names[dep] {
				return nil, errors.New(ErrMissingDep,
					fmt.Sprintf("service %q depends on %q, which is not registered", e.name, dep), nil).
					WithField("service", e.name).
					WithField("dependency", dep)
			}
		}
	}

	// Build in-degree map and adjacency list.
	inDegree := make(map[string]int, len(m.entries))
	dependents := make(map[string][]string, len(m.entries)) // dep -> services that depend on it
	for _, e := range m.entries {
		if _, ok := inDegree[e.name]; !ok {
			inDegree[e.name] = 0
		}
		for _, dep := range e.deps {
			dependents[dep] = append(dependents[dep], e.name)
			inDegree[e.name]++
		}
	}

	// Start with nodes that have no dependencies.
	var queue []string
	for _, e := range m.entries {
		if inDegree[e.name] == 0 {
			queue = append(queue, e.name)
		}
	}

	var order []string
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		order = append(order, name)

		for _, dependent := range dependents[name] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(order) != len(m.entries) {
		return nil, errors.New(ErrCyclicDependency,
			"cyclic dependency detected among services", nil)
	}

	return order, nil
}

// entryMap builds a name -> entry lookup.
func (m *Manager) entryMap() map[string]*entry {
	byName := make(map[string]*entry, len(m.entries))
	for _, e := range m.entries {
		byName[e.name] = e
	}
	return byName
}

// emit sends an event if an emitter is configured.
func (m *Manager) emit(name string, data any) {
	if m.emitter != nil {
		m.emitter.Emit(name, data)
	}
}
