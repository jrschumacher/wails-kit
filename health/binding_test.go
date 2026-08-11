package health

import "testing"

func TestBindingExposesSnapshotAndTrigger(t *testing.T) {
	r := New(WithoutDefaultConnectivityCheck())
	if _, err := r.Register(Check{Name: "api", Probe: noopProbe{}}); err != nil {
		t.Fatal(err)
	}

	b := r.Binding()

	if got := b.GetSnapshot().Checks[0].State; got != StateUnknown {
		t.Fatalf("expected unknown before Trigger, got %s", got)
	}

	b.Trigger("api")

	if got := b.GetSnapshot().Checks[0].State; got != StateHealthy {
		t.Fatalf("expected healthy after Trigger, got %s", got)
	}
}
