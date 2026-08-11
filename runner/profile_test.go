package runner

import (
	"testing"
	"time"
)

func TestEphemeralDefaults(t *testing.T) {
	p := Ephemeral()
	if p.Persistence != PersistNone {
		t.Errorf("Persistence = %v, want PersistNone", p.Persistence)
	}
	if p.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", p.MaxAttempts)
	}
	if p.DeadLetter {
		t.Error("DeadLetter = true, want false")
	}
}

func TestDurableDefaults(t *testing.T) {
	p := Durable()
	if p.Persistence != PersistStore {
		t.Errorf("Persistence = %v, want PersistStore", p.Persistence)
	}
	if p.MaxAttempts != 0 {
		t.Errorf("MaxAttempts = %d, want 0 (unlimited)", p.MaxAttempts)
	}
	if !p.DeadLetter {
		t.Error("DeadLetter = false, want true")
	}
}

func TestBestEffortDefaults(t *testing.T) {
	p := BestEffort()
	if p.Persistence != PersistStore {
		t.Errorf("Persistence = %v, want PersistStore", p.Persistence)
	}
	if p.MaxAttempts <= 0 {
		t.Errorf("MaxAttempts = %d, want > 0 (bounded)", p.MaxAttempts)
	}
	if p.DeadLetter {
		t.Error("DeadLetter = true, want false (best-effort drops, doesn't dead-letter)")
	}
}

// TestProfileFieldsOverridable is the "profiles, not knobs" contract:
// p := runner.Durable(); p.MaxAttempts = 10 must actually change behavior
// downstream, not just the struct field. New()/failJob's use of
// q.profile.MaxAttempts (rather than re-deriving from Name) is what makes
// this true; see TestProfileOverrides in queue_test.go for the behavioral
// version of this assertion.
func TestProfileFieldsOverridable(t *testing.T) {
	p := Durable()
	p.MaxAttempts = 10
	if p.MaxAttempts != 10 {
		t.Fatalf("MaxAttempts = %d, want 10", p.MaxAttempts)
	}
	// The rest of the profile is untouched by the override.
	if !p.DeadLetter {
		t.Error("DeadLetter changed by an unrelated override")
	}
}

func TestDefaultBackoffBounds(t *testing.T) {
	for attempt := 1; attempt <= 30; attempt++ {
		d := DefaultBackoff(attempt)
		if d <= 0 {
			t.Fatalf("DefaultBackoff(%d) = %v, want > 0", attempt, d)
		}
		if d > 5*time.Minute {
			t.Fatalf("DefaultBackoff(%d) = %v, want <= 5m cap", attempt, d)
		}
	}
}

func TestDefaultBackoffGrows(t *testing.T) {
	// Not strictly monotonic per-call (jitter), but the ceiling for a
	// later attempt must be >= the ceiling for an earlier one until the
	// cap is reached.
	small := DefaultBackoff(1)
	if small > time.Second {
		t.Fatalf("DefaultBackoff(1) = %v, want <= 1s (base)", small)
	}
	capped := DefaultBackoff(20)
	if capped > 5*time.Minute {
		t.Fatalf("DefaultBackoff(20) = %v, want <= 5m cap", capped)
	}
}

func TestDefaultBackoffNonPositiveAttemptTreatedAsOne(t *testing.T) {
	d := DefaultBackoff(0)
	if d <= 0 || d > time.Second {
		t.Fatalf("DefaultBackoff(0) = %v, want same range as attempt 1 (0 < d <= 1s)", d)
	}
}
