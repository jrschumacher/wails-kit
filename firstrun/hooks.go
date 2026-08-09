package firstrun

import "context"

// Hook is one unit of work to run in response to a detected transition.
// Service.Run evaluates every registered Hook, in registration order
// (WithHooks preserves call order; multiple WithHooks calls append), and
// runs it if When(info) returns true (or When is nil, meaning "always").
//
// Run functions must be idempotent: if any Hook's Run returns an error,
// Service.Run stops evaluating remaining hooks and does not update the
// version stamp — the next launch re-detects the same transition and
// re-runs every matching hook from the start, including ones that already
// succeeded this time.
type Hook struct {
	// Name identifies the hook in error messages and logs. Optional but
	// strongly recommended — an unnamed hook's failure is reported as
	// "(unnamed)".
	Name string
	// When filters whether this hook runs for a given Info. Nil means
	// "always run, for every Kind including Same" — most hooks should use
	// one of OnFresh, OnUpgrade, OnDowngrade, or OnUpgradeThrough instead of
	// leaving this nil.
	When func(Info) bool
	// Run performs the hook's work. ctx is the context passed to
	// Service.Run.
	Run func(ctx context.Context, info Info) error
}

// OnFresh returns a filter matching only Kind == Fresh.
func OnFresh() func(Info) bool {
	return func(info Info) bool { return info.Kind == Fresh }
}

// OnUpgrade returns a filter matching any Kind == Upgrade, regardless of
// the specific versions involved. Use OnUpgradeThrough instead when a hook
// only applies to upgrades that cross a particular version boundary.
func OnUpgrade() func(Info) bool {
	return func(info Info) bool { return info.Kind == Upgrade }
}

// OnDowngrade returns a filter matching only Kind == Downgrade. Downgrade
// is not an error by default (see the package doc) — pair this with a Run
// function that warns, migrates defensively, or returns ErrRefuse to
// refuse cleanly.
func OnDowngrade() func(Info) bool {
	return func(info Info) bool { return info.Kind == Downgrade }
}

// OnUpgradeThrough returns a filter matching an Upgrade whose (Previous,
// Current] range includes version — i.e. Previous < version <= Current.
// This is what makes a multi-version jump (e.g. 1.0.0 -> 1.4.0) run every
// intervening migration: register one Hook per version boundary with
// OnUpgradeThrough(that boundary), in ascending version order. Hooks run in
// registration order (not sorted by boundary), so registering them out of
// order runs them out of order — this is the caller's responsibility, not
// something this package can infer.
//
// The filter never matches Fresh, Same, or Downgrade — a fresh install
// starts at Current with nothing to migrate from, so replaying every
// historical migration against it would be wrong.
//
// version must be a valid semver string; an invalid one is a programmer
// error (the boundary is normally a literal in source) and OnUpgradeThrough
// panics rather than silently never matching, which would be far harder to
// notice.
func OnUpgradeThrough(version string) func(Info) bool {
	boundary := parseVersionOrPanic("OnUpgradeThrough", version)
	return func(info Info) bool {
		if info.Kind != Upgrade {
			return false
		}
		return info.Previous.Compare(boundary) < 0 && boundary.Compare(info.Current) <= 0
	}
}
