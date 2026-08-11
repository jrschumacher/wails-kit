package firstrun

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/semver"
	"github.com/jrschumacher/wails-kit/v2/state"
)

// stamp is the on-disk record of the last version Service.Run completed
// successfully for. An empty Version means "no stamp has ever been
// written" — Detect relies on this to distinguish a missing file (state.
// Store.Load returns the zero stamp with no error either way) from a
// genuinely recorded version. Service never writes a stamp with an empty
// Version, so this is an unambiguous signal.
type stamp struct {
	Version string `json:"version"`
}

// Service detects version transitions and runs Hooks in response. Build
// one with New.
type Service struct {
	mu sync.Mutex

	current  semver.Version
	baseline *semver.Version // set by WithBaselineVersion; nil means "none"
	hooks    []Hook
	emitter  *events.Emitter
	store    *state.Store[stamp]
}

// Option configures a Service. See New.
type Option func(*serviceConfig)

// serviceConfig accumulates Option values before New validates and builds
// the Service and its backing state.Store — kept separate from Service so
// New can validate raw inputs (an appName string, a version string) before
// any parsing happens.
type serviceConfig struct {
	appName     string
	dirs        *appdirs.Dirs
	storagePath string
	versionStr  string
	baselineStr string
	emitter     *events.Emitter
	hooks       []Hook
}

// WithAppName sets the storage location using appdirs.New(name) — the
// version stamp is stored at {dataDir}/state/firstrun.json. Mutually
// exclusive in effect with WithDirs/WithStoragePath; whichever was set last
// among the three wins if more than one is passed.
func WithAppName(name string) Option {
	return func(c *serviceConfig) {
		c.appName = name
		c.dirs = nil
		c.storagePath = ""
	}
}

// WithDirs sets the storage location from an already-constructed
// appdirs.Dirs (e.g. one shared with other kit services) instead of
// deriving a fresh one from an app name. See WithAppName for precedence.
func WithDirs(d *appdirs.Dirs) Option {
	return func(c *serviceConfig) {
		c.dirs = d
		c.appName = ""
		c.storagePath = ""
	}
}

// WithStoragePath overrides the full file path for the version stamp,
// bypassing appdirs entirely. Primarily for tests (pair with t.TempDir()).
// See WithAppName for precedence.
func WithStoragePath(path string) Option {
	return func(c *serviceConfig) {
		c.storagePath = path
		c.appName = ""
		c.dirs = nil
	}
}

// WithVersion sets the app's current version (e.g. "1.4.0"), the Current
// value in every Info this Service produces. Required — New returns
// ErrConfig without it.
func WithVersion(version string) Option {
	return func(c *serviceConfig) { c.versionStr = version }
}

// WithBaselineVersion addresses adopting firstrun mid-life: an app with
// existing users has installs with no recorded stamp that are emphatically
// not fresh installs. Without this option, Detect cannot tell the
// difference and reports Fresh for both.
//
// When set, a missing stamp is treated as if the last successful run was
// baseline rather than "no prior version" — Detect then reports Upgrade
// (or Same, if baseline == the app's current version) instead of Fresh, and
// Upgrade/OnUpgradeThrough hooks between baseline and Current fire exactly
// as they would for a user who really was on baseline. Pick baseline once,
// as the last version released before this package was adopted, and never
// change it — it is a fixed historical marker, not something to bump per
// release.
//
// Limitation: an app that never sets this treats every pre-adoption
// install as Fresh the first time it runs with firstrun. That silently
// re-runs first-run onboarding (and skips every upgrade hook) for existing
// users. Document this loudly for any app that skips WithBaselineVersion.
func WithBaselineVersion(version string) Option {
	return func(c *serviceConfig) { c.baselineStr = version }
}

// WithEmitter wires an events.Emitter so EventTransition fires after a
// successful Run. Nil (the default) means transitions are silent.
func WithEmitter(e *events.Emitter) Option {
	return func(c *serviceConfig) { c.emitter = e }
}

// WithHooks appends hooks to the Service's hook list, in the order given.
// Safe to call more than once; later calls append rather than replace.
// Registration order is evaluation order (see Hook, OnUpgradeThrough).
func WithHooks(hooks ...Hook) Option {
	return func(c *serviceConfig) { c.hooks = append(c.hooks, hooks...) }
}

// New builds a Service. It returns ErrConfig if WithVersion was not
// provided, if the version (or WithBaselineVersion's version) does not
// parse as semver, or if none of WithAppName/WithDirs/WithStoragePath was
// provided (state.Store has nowhere to persist the stamp otherwise).
func New(opts ...Option) (*Service, error) {
	c := &serviceConfig{}
	for _, opt := range opts {
		opt(c)
	}

	if c.versionStr == "" {
		return nil, errors.New(ErrConfig, "firstrun.New requires WithVersion", nil)
	}
	current, err := semver.ParseVersion(c.versionStr)
	if err != nil {
		return nil, errors.Wrap(ErrConfig, "firstrun.New: WithVersion is not a valid semver version", err)
	}

	var baseline *semver.Version
	if c.baselineStr != "" {
		b, err := semver.ParseVersion(c.baselineStr)
		if err != nil {
			return nil, errors.Wrap(ErrConfig, "firstrun.New: WithBaselineVersion is not a valid semver version", err)
		}
		baseline = &b
	}

	var stateOpts []state.Option[stamp]
	switch {
	case c.storagePath != "":
		stateOpts = append(stateOpts, state.WithStoragePath[stamp](c.storagePath))
	case c.dirs != nil:
		path := filepath.Join(c.dirs.Data(), "state", "firstrun.json")
		stateOpts = append(stateOpts, state.WithStoragePath[stamp](path))
	case c.appName != "":
		stateOpts = append(stateOpts,
			state.WithAppName[stamp](c.appName),
			state.WithName[stamp]("firstrun"),
		)
	default:
		return nil, errors.New(ErrConfig, "firstrun.New requires WithAppName, WithDirs, or WithStoragePath", nil)
	}

	store, err := state.New(stateOpts...)
	if err != nil {
		return nil, err
	}

	return &Service{
		current:  current,
		baseline: baseline,
		hooks:    c.hooks,
		emitter:  c.emitter,
		store:    store,
	}, nil
}

// Detect reads the recorded version stamp and classifies the transition
// without running any Hook or writing anything. Run calls this internally;
// call it directly to inspect what Run would do (e.g. to decide whether to
// show a "what's new" screen) without side effects.
func (s *Service) Detect() (Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.detect()
}

func (s *Service) detect() (Info, error) {
	st, recovered, err := s.store.LoadDetailed()
	if err != nil {
		return Info{}, err
	}

	if recovered {
		// The stamp file existed but failed to parse as JSON; state.Store
		// has already quarantined it (preserving the bytes for
		// support/debugging) and returned a zero stamp in its place. That
		// zero stamp is indistinguishable from "no stamp ever recorded" by
		// value alone — but treating it as Fresh would re-run OnFresh
		// onboarding hooks for what is almost certainly an existing user
		// whose stamp merely got corrupted, which is worse than doing
		// nothing. It's equally wrong to guess a "previous" version and run
		// upgrade migrations against a fabrication.
		//
		// So corruption is deliberately its own outcome, distinct from both
		// Fresh and Upgrade: it reports Same (current -> current), which
		// runs only hooks with no When filter (or an explicit Same
		// interest) and none of OnFresh/OnUpgrade/OnDowngrade. Run then
		// writes a fresh, valid stamp at the current version, so the next
		// launch behaves normally again — this one launch is the full
		// extent of the recovery.
		return Info{Kind: Same, Previous: s.current, Current: s.current}, nil
	}

	if st.Version == "" {
		if s.baseline != nil {
			return classify(*s.baseline, s.current), nil
		}
		return Info{Kind: Fresh, Current: s.current}, nil
	}

	prev, err := semver.ParseVersion(st.Version)
	if err != nil {
		return Info{}, errors.Wrap(ErrStamp, fmt.Sprintf("firstrun: recorded version stamp %q is invalid", st.Version), err)
	}
	return classify(prev, s.current), nil
}

func classify(previous, current semver.Version) Info {
	info := Info{Previous: previous, Current: current}
	switch previous.Compare(current) {
	case 0:
		info.Kind = Same
	case -1:
		info.Kind = Upgrade
	default:
		info.Kind = Downgrade
	}
	return info
}

// Run detects the transition, then runs every Hook whose When(info)
// matches, in registration order. If every matching Hook succeeds, Run
// records Current as the new stamp and — if an emitter was configured —
// emits EventTransition, then returns the detected Info.
//
// If a Hook's Run function returns an error, Run stops immediately: no
// later hook runs, and the stamp is not updated (documented in the package
// doc: the next call to Run will re-detect the same transition and re-run
// every matching hook, including ones that already succeeded this time —
// hooks must be idempotent). The returned error wraps the hook's error as
// ErrHookFailed; errors.Is/As and errors.Unwrap see through it to the
// original error.
func (s *Service) Run(ctx context.Context) (Info, error) {
	info, err := s.run(ctx)
	if err != nil {
		return info, err
	}
	s.emit(info)
	return info, nil
}

// run does one Run call's work. It holds s.mu only long enough to detect
// the transition and snapshot the hook list (detectAndSnapshotHooks), then
// releases the lock before invoking any Hook.Run or state.Store.Save.
//
// Hooks are arbitrary caller code, and s.mu is a plain sync.Mutex — not
// re-entrant. The previous implementation ran the entire hook loop (and the
// final Save) inside the same lock scope used by Detect and Run, so a hook
// that called back into Detect or Run on this same Service — a natural
// thing for a migration hook to do, e.g. to check the transition again —
// deadlocked forever. state.Store, settings.Service, and appearance all
// have (or narrowly avoided) this exact bug; this mirrors their fix:
// snapshot under the lock, then do everything external with it released.
// See TestHookReentryDoesNotDeadlock.
func (s *Service) run(ctx context.Context) (Info, error) {
	info, hooks, err := s.detectAndSnapshotHooks()
	if err != nil {
		return Info{}, err
	}

	for _, h := range hooks {
		if h.When != nil && !h.When(info) {
			continue
		}
		if h.Run == nil {
			continue
		}
		if err := h.Run(ctx, info); err != nil {
			name := h.Name
			if name == "" {
				name = "(unnamed)"
			}
			return info, errors.Wrap(ErrHookFailed, fmt.Sprintf("firstrun: hook %q failed", name), err)
		}
	}

	if err := s.store.Save(stamp{Version: s.current.String()}); err != nil {
		return info, err
	}

	return info, nil
}

// detectAndSnapshotHooks holds s.mu just long enough to detect the
// transition and copy the hook list, then releases it before returning.
// The hook list is only ever set at construction (WithHooks), so copying it
// is defensive rather than strictly required today — but it keeps this
// function honest about what "snapshot under the lock" means, and protects
// against a future API (e.g. an AddHook, mirroring settings.AddOnChange)
// silently reintroducing a read/write race.
func (s *Service) detectAndSnapshotHooks() (Info, []Hook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, err := s.detect()
	if err != nil {
		return Info{}, nil, err
	}
	hooks := make([]Hook, len(s.hooks))
	copy(hooks, s.hooks)
	return info, hooks, nil
}

// Reset deletes the recorded version stamp, so the next Detect or Run
// treats the app exactly as if it had never run before (Fresh, or Upgrade
// from WithBaselineVersion if that was set) — including re-running OnFresh
// hooks. This is the explicit, user-triggerable counterpart to the
// automatic corruption recovery in detect: a corrupt stamp already heals
// itself on the next launch without anyone calling this, but an app may
// still want to expose an explicit "repair my installation" action (a
// support flow, a CLI flag) that does not require a user to find and delete
// the stamp file by hand — the stamp's path is otherwise internal to this
// package.
func (s *Service) Reset() error {
	return s.store.Delete()
}

// emit is called after runLocked has returned and s.mu is released — never
// emit while holding a lock (see AGENTS.md Invariants and state/AGENTS.md,
// which this mirrors): a handler that calls back into Detect/Run on this
// same Service must not deadlock against a lock this goroutine still held
// a moment ago.
func (s *Service) emit(info Info) {
	if s.emitter == nil {
		return
	}
	previous := ""
	if info.Kind != Fresh {
		previous = info.Previous.String()
	}
	s.emitter.Emit(EventTransition, TransitionPayload{
		Kind:     info.Kind,
		Previous: previous,
		Current:  info.Current.String(),
	})
}

// parseVersionOrPanic is used by OnUpgradeThrough, whose version boundary
// is normally a source-code literal — an unparseable literal is a
// programmer error caught the first time the filter is exercised, not
// something that should silently never match (see appdirs.New for the same
// "panic on a caller-supplied literal that is structurally wrong"
// convention elsewhere in the kit).
func parseVersionOrPanic(fn, version string) semver.Version {
	v, err := semver.ParseVersion(version)
	if err != nil {
		panic(fmt.Sprintf("firstrun.%s: invalid version %q: %v", fn, version, err))
	}
	return v
}
