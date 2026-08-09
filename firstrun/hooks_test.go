package firstrun

import (
	"context"
	"errors"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/semver"
)

func mustVersion(t *testing.T, s string) semver.Version {
	t.Helper()
	v, err := semver.ParseVersion(s)
	if err != nil {
		t.Fatalf("semver.ParseVersion(%q): %v", s, err)
	}
	return v
}

func TestOnFreshOnUpgradeOnDowngradeFilters(t *testing.T) {
	fresh := Info{Kind: Fresh}
	upgrade := Info{Kind: Upgrade}
	downgrade := Info{Kind: Downgrade}
	same := Info{Kind: Same}

	if !OnFresh()(fresh) || OnFresh()(upgrade) || OnFresh()(downgrade) || OnFresh()(same) {
		t.Fatal("OnFresh must match only Kind == Fresh")
	}
	if !OnUpgrade()(upgrade) || OnUpgrade()(fresh) || OnUpgrade()(downgrade) || OnUpgrade()(same) {
		t.Fatal("OnUpgrade must match only Kind == Upgrade")
	}
	if !OnDowngrade()(downgrade) || OnDowngrade()(fresh) || OnDowngrade()(upgrade) || OnDowngrade()(same) {
		t.Fatal("OnDowngrade must match only Kind == Downgrade")
	}
}

// TestUpgradeThroughBoundary is the requirement-1/5 regression: the filter
// must implement Previous < version <= Current, never match outside Kind
// == Upgrade, and never match a Fresh install (nothing to migrate from).
func TestUpgradeThroughBoundary(t *testing.T) {
	tests := []struct {
		name     string
		info     Info
		boundary string
		want     bool
	}{
		{
			name:     "boundary strictly inside range",
			info:     Info{Kind: Upgrade, Previous: mustVersion(t, "1.0.0"), Current: mustVersion(t, "1.4.0")},
			boundary: "1.2.0",
			want:     true,
		},
		{
			name:     "boundary equals current is inclusive",
			info:     Info{Kind: Upgrade, Previous: mustVersion(t, "1.0.0"), Current: mustVersion(t, "1.4.0")},
			boundary: "1.4.0",
			want:     true,
		},
		{
			name:     "boundary equals previous is exclusive",
			info:     Info{Kind: Upgrade, Previous: mustVersion(t, "1.2.0"), Current: mustVersion(t, "1.4.0")},
			boundary: "1.2.0",
			want:     false,
		},
		{
			name:     "boundary beyond current does not match",
			info:     Info{Kind: Upgrade, Previous: mustVersion(t, "1.0.0"), Current: mustVersion(t, "1.4.0")},
			boundary: "1.5.0",
			want:     false,
		},
		{
			name:     "boundary below previous does not match",
			info:     Info{Kind: Upgrade, Previous: mustVersion(t, "1.2.0"), Current: mustVersion(t, "1.4.0")},
			boundary: "1.1.0",
			want:     false,
		},
		{
			name:     "prerelease ordering from semver applies",
			info:     Info{Kind: Upgrade, Previous: mustVersion(t, "2.0.0-beta.1"), Current: mustVersion(t, "2.0.0")},
			boundary: "2.0.0",
			want:     true,
		},
		{
			name:     "never matches Fresh even when versions would overlap",
			info:     Info{Kind: Fresh, Previous: semver.Version{}, Current: mustVersion(t, "1.4.0")},
			boundary: "1.0.0",
			want:     false,
		},
		{
			name:     "never matches Downgrade",
			info:     Info{Kind: Downgrade, Previous: mustVersion(t, "1.4.0"), Current: mustVersion(t, "1.0.0")},
			boundary: "1.2.0",
			want:     false,
		},
		{
			name:     "never matches Same",
			info:     Info{Kind: Same, Previous: mustVersion(t, "1.0.0"), Current: mustVersion(t, "1.0.0")},
			boundary: "1.0.0",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OnUpgradeThrough(tt.boundary)(tt.info)
			if got != tt.want {
				t.Errorf("OnUpgradeThrough(%s)(%+v) = %v, want %v", tt.boundary, tt.info, got, tt.want)
			}
		})
	}
}

func TestOnUpgradeThroughPanicsOnInvalidVersion(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected OnUpgradeThrough to panic on an invalid version literal")
		}
	}()
	OnUpgradeThrough("not-a-version")
}

// TestHookOrder is the requirement-1 regression: a user upgrading across
// several versions in one jump must run every intervening hook, in
// registration order, not just the last one.
func TestHookOrder(t *testing.T) {
	var ran []string
	record := func(name string) Hook {
		return Hook{
			Name: name,
			When: OnUpgradeThrough(name),
			Run: func(context.Context, Info) error {
				ran = append(ran, name)
				return nil
			},
		}
	}

	path := t.TempDir() + "/firstrun.json"
	seed, err := New(WithVersion("1.0.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := seed.Run(context.Background()); err != nil {
		t.Fatalf("seed Run: %v", err)
	}

	svc, err := New(
		WithVersion("1.4.0"),
		WithStoragePath(path),
		WithHooks(
			record("1.1.0"),
			record("1.2.0"),
			record("1.3.0"),
			record("1.4.0"),
			record("1.5.0"), // beyond Current — must not fire
		),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	info, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if info.Kind != Upgrade {
		t.Fatalf("Kind = %v, want Upgrade", info.Kind)
	}

	want := []string{"1.1.0", "1.2.0", "1.3.0", "1.4.0"}
	if len(ran) != len(want) {
		t.Fatalf("ran = %v, want %v", ran, want)
	}
	for i := range want {
		if ran[i] != want[i] {
			t.Fatalf("ran = %v, want %v", ran, want)
		}
	}
}

// TestUpgradeStopsAtFirstFailure checks that a mid-sequence failure halts
// remaining hooks (part of requirement 2's "no half-completed migration is
// mistaken for a completed one").
func TestUpgradeStopsAtFirstFailure(t *testing.T) {
	var ran []string
	boom := errors.New("boom")

	path := t.TempDir() + "/firstrun.json"
	seed, err := New(WithVersion("1.0.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := seed.Run(context.Background()); err != nil {
		t.Fatalf("seed Run: %v", err)
	}

	svc, err := New(
		WithVersion("1.3.0"),
		WithStoragePath(path),
		WithHooks(
			Hook{Name: "1.1.0", When: OnUpgradeThrough("1.1.0"), Run: func(context.Context, Info) error {
				ran = append(ran, "1.1.0")
				return nil
			}},
			Hook{Name: "1.2.0", When: OnUpgradeThrough("1.2.0"), Run: func(context.Context, Info) error {
				ran = append(ran, "1.2.0")
				return boom
			}},
			Hook{Name: "1.3.0", When: OnUpgradeThrough("1.3.0"), Run: func(context.Context, Info) error {
				ran = append(ran, "1.3.0")
				return nil
			}},
		),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := svc.Run(context.Background()); err == nil {
		t.Fatal("expected Run to fail")
	}
	if len(ran) != 2 || ran[0] != "1.1.0" || ran[1] != "1.2.0" {
		t.Fatalf("ran = %v, want [1.1.0 1.2.0] (must stop before 1.3.0)", ran)
	}

	// The stamp must still read 1.0.0 — the failed hook must not have
	// caused the upgrade to be considered even partially applied.
	info, err := svc.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Kind != Upgrade || info.Previous.String() != "v1.0.0" {
		t.Fatalf("Detect = %+v, want Upgrade from v1.0.0 (stamp must be unchanged)", info)
	}
}

// TestDowngradeRefusal is the requirement-3 regression: downgrade is not an
// error by default, but a Hook using OnDowngrade can refuse cleanly via
// ErrRefuse, and refusal must be identifiable with errors.Is even after Run
// wraps it.
func TestDowngradeRefusal(t *testing.T) {
	path := t.TempDir() + "/firstrun.json"
	seed, err := New(WithVersion("2.0.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := seed.Run(context.Background()); err != nil {
		t.Fatalf("seed Run: %v", err)
	}

	svc, err := New(
		WithVersion("1.0.0"),
		WithStoragePath(path),
		WithHooks(Hook{
			Name: "refuse-downgrade",
			When: OnDowngrade(),
			Run: func(context.Context, Info) error {
				return ErrRefuse
			},
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, runErr := svc.Run(context.Background())
	if runErr == nil {
		t.Fatal("expected Run to return the refusal error")
	}
	if !errors.Is(runErr, ErrRefuse) {
		t.Fatalf("errors.Is(err, ErrRefuse) = false, err = %v", runErr)
	}

	// Stamp must remain at 2.0.0 — the downgrade was refused, not applied.
	info, err := svc.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Kind != Downgrade || info.Previous.String() != "v2.0.0" {
		t.Fatalf("Detect = %+v, want Downgrade from v2.0.0", info)
	}
}

// TestDowngradeWithoutHookProceeds checks requirement 3's other half:
// downgrade is a real case, not an error case, by default — with no
// OnDowngrade hook registered, Run succeeds and records the (lower)
// version.
func TestDowngradeWithoutHookProceeds(t *testing.T) {
	path := t.TempDir() + "/firstrun.json"
	seed, err := New(WithVersion("2.0.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := seed.Run(context.Background()); err != nil {
		t.Fatalf("seed Run: %v", err)
	}

	svc, err := New(WithVersion("1.0.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	info, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if info.Kind != Downgrade {
		t.Fatalf("Kind = %v, want Downgrade", info.Kind)
	}

	// A subsequent launch at the same (lower) version now sees Same.
	relaunch, err := New(WithVersion("1.0.0"), WithStoragePath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	info, err = relaunch.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Kind != Same {
		t.Fatalf("Kind = %v, want Same", info.Kind)
	}
}

// TestAdoptedMidLife is the requirement-4 regression: an app adopting
// firstrun after already shipping must not treat every existing install
// (no stamp on disk) as Fresh when WithBaselineVersion is set.
func TestAdoptedMidLife(t *testing.T) {
	t.Run("baseline older than current reports Upgrade, not Fresh", func(t *testing.T) {
		svc := newTestService(t, WithVersion("1.5.0"), WithBaselineVersion("1.2.0"))
		info, err := svc.Detect()
		if err != nil {
			t.Fatalf("Detect: %v", err)
		}
		if info.Kind != Upgrade {
			t.Fatalf("Kind = %v, want Upgrade", info.Kind)
		}
		if info.Previous.String() != "v1.2.0" {
			t.Fatalf("Previous = %v, want v1.2.0", info.Previous)
		}
	})

	t.Run("baseline equal to current reports Same, not Fresh", func(t *testing.T) {
		svc := newTestService(t, WithVersion("1.5.0"), WithBaselineVersion("1.5.0"))
		info, err := svc.Detect()
		if err != nil {
			t.Fatalf("Detect: %v", err)
		}
		if info.Kind != Same {
			t.Fatalf("Kind = %v, want Same", info.Kind)
		}
	})

	t.Run("upgrade-through hooks between baseline and current fire for an adopted install", func(t *testing.T) {
		var ran []string
		svc := newTestService(t,
			WithVersion("1.5.0"),
			WithBaselineVersion("1.2.0"),
			WithHooks(
				Hook{Name: "1.3.0", When: OnUpgradeThrough("1.3.0"), Run: func(context.Context, Info) error {
					ran = append(ran, "1.3.0")
					return nil
				}},
				Hook{Name: "1.5.0", When: OnUpgradeThrough("1.5.0"), Run: func(context.Context, Info) error {
					ran = append(ran, "1.5.0")
					return nil
				}},
			),
		)
		if _, err := svc.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if len(ran) != 2 || ran[0] != "1.3.0" || ran[1] != "1.5.0" {
			t.Fatalf("ran = %v, want [1.3.0 1.5.0]", ran)
		}
	})

	t.Run("without a baseline a missing stamp is Fresh (the documented limitation)", func(t *testing.T) {
		svc := newTestService(t, WithVersion("1.5.0"))
		info, err := svc.Detect()
		if err != nil {
			t.Fatalf("Detect: %v", err)
		}
		if info.Kind != Fresh {
			t.Fatalf("Kind = %v, want Fresh", info.Kind)
		}
	})
}
