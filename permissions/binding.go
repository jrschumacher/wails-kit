package permissions

import "context"

// Binding is the frontend-safe surface of Service, following the same
// split settings.Service/health.Registry use: Wails v3 binds every
// exported method of a registered service, so register only Binding, never
// *Service directly.
type Binding struct {
	s *Service
}

// Binding returns the frontend-safe binding surface for s. Register the
// returned value with Wails (application.NewService(s.Binding())).
func (s *Service) Binding() *Binding {
	return &Binding{s: s}
}

// Check reports the current authorization state for k. See Service.Check.
func (b *Binding) Check(k Kind) Status {
	return b.s.Check(k)
}

// Request triggers the OS-level authorization flow for k. It calls
// Service.Request with context.Background(): a webview RPC call has no
// mechanism to hand this binding a cancellable Go context, so a frontend
// that needs a "give up waiting" UI does so by not awaiting the promise
// rather than by cancellation, and re-checks with Check when it comes
// back. See Service.Request for the full contract.
func (b *Binding) Request(k Kind) (Status, error) {
	return b.s.Request(context.Background(), k)
}

// OpenSystemSettings opens the OS settings pane relevant to k. See
// Service.OpenSystemSettings.
func (b *Binding) OpenSystemSettings(k Kind) error {
	return b.s.OpenSystemSettings(k)
}
