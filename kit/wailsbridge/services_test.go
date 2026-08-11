package wailsbridge

import "testing"

// TestServices_MatchesBindingInstanceCount verifies Services(k) returns
// one application.Service per entry bindingInstances would build — the
// alternative path for a developer who passes
// application.Options{Services: wailsbridge.Services(k)} to application.New
// instead of letting Attach call app.RegisterService.
func TestServices_MatchesBindingInstanceCount(t *testing.T) {
	k := newTestKit(t)

	svcs := Services(k)
	instances := bindingInstances(k, newConfig())

	if len(svcs) != len(instances) {
		t.Errorf("len(Services(k)) = %d, want %d (len(bindingInstances))", len(svcs), len(instances))
	}
}
