package settings

// Binding is the frontend-safe surface of Service. It is the ONLY type that
// should ever be registered as a Wails service — Wails v3 binds every
// exported method of a registered service, so registering *Service directly
// would publish GetSecret (raw, unmasked API keys) to webview JS and
// nullify SecretMask entirely.
//
// Binding exposes exactly GetSchema, GetValues, and SetValues. Anything
// that reads or writes a raw secret (GetSecret) stays on *Service, for
// backend Go callers only.
type Binding struct {
	svc *Service
}

// Binding returns the frontend-safe binding surface for s. Register the
// returned value with Wails (application.NewService(svc.Binding())) —
// never register s itself.
func (s *Service) Binding() *Binding {
	return &Binding{svc: s}
}

// GetSchema returns the settings schema, resolved to plain strings against
// the service's localizer. See Service.GetSchema.
func (b *Binding) GetSchema() ResolvedSchema {
	return b.svc.GetSchema()
}

// GetValues returns all current settings values, with password fields
// masked. See Service.GetValues.
func (b *Binding) GetValues() (map[string]any, error) {
	return b.svc.GetValues()
}

// SetValues validates and saves settings. See Service.SetValues.
func (b *Binding) SetValues(values map[string]any) ([]ValidationError, error) {
	return b.svc.SetValues(values)
}
