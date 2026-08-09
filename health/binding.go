package health

// Binding is the frontend-safe surface of Registry, following the same
// split settings.Service and i18n.Localizer use: Wails v3 binds every
// exported method of a registered service, so register only Binding, never
// *Registry directly, once Registry grows backend-only methods (e.g.
// Register/Unregister, which take Go-only Probe values and have no
// business being webview-callable).
type Binding struct {
	r *Registry
}

// Binding returns the frontend-safe binding surface for r. Register the
// returned value with Wails (application.NewService(r.Binding())).
func (r *Registry) Binding() *Binding {
	return &Binding{r: r}
}

// GetSnapshot returns the registry's current resolved view. See
// Registry.Snapshot.
func (b *Binding) GetSnapshot() Snapshot {
	return b.r.Snapshot()
}

// Trigger probes the named checks immediately. See Registry.Trigger.
func (b *Binding) Trigger(names ...string) {
	b.r.Trigger(names...)
}
