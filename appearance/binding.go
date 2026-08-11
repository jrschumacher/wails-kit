package appearance

// Binding is the frontend-safe surface of Service, following the same split
// settings.Service and i18n.Localizer use (settings/binding.go,
// i18n/binding.go): Wails v3 binds every exported method of a registered
// service, so register only Binding with Wails, never *Service directly.
// Every Service method happens to be frontend-safe today, but the split is
// the load-bearing convention the rest of the kit (and wailsbridge) relies
// on, so it starts here rather than being bolted on later.
type Binding struct {
	svc *Service
}

// Binding returns the frontend-safe binding surface for s. Register the
// returned value with Wails (application.NewService(svc.Binding())) — never
// register s itself.
func (s *Service) Binding() *Binding {
	return &Binding{svc: s}
}

// GetMode returns the current user preference. See Service.Mode.
func (b *Binding) GetMode() Mode {
	return b.svc.Mode()
}

// SetMode validates, persists (if a settings.Service is wired), and applies
// a new mode. See Service.SetMode.
func (b *Binding) SetMode(m Mode) error {
	return b.svc.SetMode(m)
}

// GetResolved returns the current resolved Theme. See Service.Resolved.
func (b *Binding) GetResolved() Theme {
	return b.svc.Resolved()
}
