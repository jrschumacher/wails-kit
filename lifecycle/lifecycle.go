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
	"time"

	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
)

// Error codes for lifecycle operations.
const (
	ErrCyclicDependency errors.Code = "lifecycle_cyclic_dependency"
	ErrMissingDep       errors.Code = "lifecycle_missing_dependency"
	ErrStartup          errors.Code = "lifecycle_startup"
	ErrShutdown         errors.Code = "lifecycle_shutdown"
	ErrTimeout          errors.Code = "lifecycle_timeout"
)

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrCyclicDependency: "Service configuration error: circular dependency detected.",
		ErrMissingDep:       "Service configuration error: a required dependency is missing.",
		ErrStartup:          "Failed to start a required service. Please try restarting the application.",
		ErrShutdown:         "An error occurred while shutting down. Some resources may not have been cleaned up.",
		ErrTimeout:          "A service took too long to respond. Please try restarting the application.",
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
	FailedService    string   `json:"failedService"`
	RollingBack      []string `json:"rollingBack"`
	RollbackErrors   []string `json:"rollbackErrors,omitempty"`
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
	Name      string `json:"name"`
	Phase     string `json:"phase"` // "startup" or "shutdown"
	Timeout   string `json:"timeout"`
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

// Manager manages ordered startup and shutdown of services.
type Manager struct {
	entries []*entry
	order   []string // topologically sorted service names
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
func (m *Manager) Startup(ctx context.Context) error {
	byName := m.entryMap()
	m.started = nil

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

			if rollbackErrs != nil {
				return stderrors.Join(startErr, rollbackErrs)
			}
			return startErr
		}

		m.started = append(m.started, name)
		m.emit(EventStarted, ServiceStartedPayload{Name: name})
	}

	return nil
}

// Shutdown stops all started services in reverse startup order.
// It does not stop on the first error; all errors are collected and joined.
func (m *Manager) Shutdown() error {
	byName := m.entryMap()
	var errs []error

	// Shut down in reverse startup order.
	for i := len(m.started) - 1; i >= 0; i-- {
		name := m.started[i]
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

	m.started = nil

	if len(errs) > 0 {
		return stderrors.Join(errs...)
	}
	return nil
}

// Health returns the health status of all started services. Services that
// implement HealthChecker report their own status; others are reported as healthy.
func (m *Manager) Health() []ServiceHealth {
	byName := m.entryMap()
	result := make([]ServiceHealth, 0, len(m.started))

	for _, name := range m.started {
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
		return errors.New(ErrTimeout,
			fmt.Sprintf("service %q startup timed out after %s", e.name, timeout), startCtx.Err()).
			WithField("service", e.name).
			WithField("timeout", timeout.String())
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
func (m *Manager) rollback(failedName string) error {
	byName := m.entryMap()

	rollingBack := make([]string, len(m.started))
	copy(rollingBack, m.started)
	// Reverse for display.
	for i, j := 0, len(rollingBack)-1; i < j; i, j = i+1, j-1 {
		rollingBack[i], rollingBack[j] = rollingBack[j], rollingBack[i]
	}

	var errs []error
	var errMsgs []string

	for _, name := range rollingBack {
		e := byName[name]
		if err := e.service.OnShutdown(); err != nil {
			errs = append(errs, errors.Wrap(ErrShutdown, fmt.Sprintf("rollback: service %q failed to shut down", name), err))
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", name, err))
		}
	}

	m.emit(EventRollback, RollbackPayload{
		FailedService:  failedName,
		RollingBack:    rollingBack,
		RollbackErrors: errMsgs,
	})

	m.started = nil

	if len(errs) > 0 {
		return stderrors.Join(errs...)
	}
	return nil
}

// topoSort performs a topological sort using Kahn's algorithm.
// Returns an error on missing dependencies or cycles.
func (m *Manager) topoSort() ([]string, error) {
	names := make(map[string]bool, len(m.entries))
	for _, e := range m.entries {
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
