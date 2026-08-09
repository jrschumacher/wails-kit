package health

import (
	"context"
	"sync"
	"time"
)

// Start runs the registry's probing loop: every registered check is probed
// immediately, then again every check.Interval (or the registry's default
// interval — see WithDefaultInterval) until ctx is cancelled.
//
// Start blocks until ctx is Done and every probe goroutine it started has
// returned — call it in its own goroutine (go r.Start(ctx)) and cancel ctx
// to stop it. A second, concurrent call to Start while one is already
// running is a no-op that returns immediately.
//
// Checks registered after Start begins join the loop immediately (see
// Register); checks removed via Unregister stop independently of ctx.
func (r *Registry) Start(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.runCtx = ctx

	names := make([]string, len(r.order))
	copy(names, r.order)
	stopChans := make(map[string]chan struct{}, len(names))
	for _, name := range names {
		stopChans[name] = r.checks[name].stopCh
	}
	r.mu.Unlock()

	for _, name := range names {
		r.wg.Add(1)
		go r.runCheck(ctx, name, stopChans[name])
	}

	<-ctx.Done()
	r.wg.Wait()

	r.mu.Lock()
	r.running = false
	r.runCtx = nil
	r.mu.Unlock()
}

// runCheck is one check's probe loop: immediate probe, then probe on every
// tick, until ctx is Done or stopCh is closed (Unregister). It always calls
// wg.Done on the Registry it belongs to, whether reached via Start's
// initial fan-out or Register's while-running fan-out.
func (r *Registry) runCheck(ctx context.Context, name string, stopCh <-chan struct{}) {
	defer r.wg.Done()

	r.mu.Lock()
	e, ok := r.checks[name]
	r.mu.Unlock()
	if !ok {
		return
	}

	interval := e.check.Interval
	if interval <= 0 {
		interval = r.defaultInterval
	}

	r.probeOnce(ctx, name)

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-t.C:
			r.probeOnce(ctx, name)
		}
	}
}

// probeOnce runs check name's Probe once (bounded by parent's cancellation;
// per-request timeouts are each Probe implementation's own responsibility —
// see HTTPProbe), records the resulting CheckStatus, and — only if State
// actually changed — emits EventChanged. A check removed between being
// scheduled and probeOnce running is silently skipped.
func (r *Registry) probeOnce(parent context.Context, name string) {
	r.mu.Lock()
	e, ok := r.checks[name]
	if !ok {
		r.mu.Unlock()
		return
	}
	check := e.check
	r.mu.Unlock()

	start := time.Now()
	err := check.Probe.Probe(parent)
	latency := time.Since(start)

	newState := StateHealthy
	errMsg := ""
	if err != nil {
		newState = StateDown
		errMsg = err.Error()
	}

	r.mu.Lock()
	e, ok = r.checks[name]
	if !ok {
		r.mu.Unlock()
		return
	}
	prevState := e.status.State
	e.status = CheckStatus{
		Name:      name,
		Class:     check.Class,
		State:     newState,
		Critical:  check.Critical,
		Err:       errMsg,
		CheckedAt: time.Now(),
		Latency:   latency,
	}
	changed := prevState != newState
	changedStatus := e.status
	var snap Snapshot
	if changed {
		snap = r.snapshotLocked()
	}
	r.mu.Unlock()

	if changed && r.emitter != nil {
		r.emitter.Emit(EventChanged, ChangedPayload{Check: changedStatus, Overall: snap.Overall})
	}
}

// Trigger probes the named checks immediately, blocking until every probe
// has completed (each still bounded by its own Probe implementation's
// timeout — see HTTPProbe), regardless of their configured Interval or
// whether Start is running. Unknown names are silently ignored. Use this
// for a user-initiated "check now" action.
func (r *Registry) Trigger(names ...string) {
	var wg sync.WaitGroup
	for _, name := range names {
		r.mu.Lock()
		_, ok := r.checks[name]
		r.mu.Unlock()
		if !ok {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			r.probeOnce(context.Background(), name)
		}(name)
	}
	wg.Wait()
}
