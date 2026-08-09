package appearance

// Source is the OS theme signal Resolved() consults for ModeSystem. It is
// intentionally Wails-free (AD-4, docs/v2-roadmap.md): appearance must be
// usable from a CLI or TUI process with no GUI, so the OS-detection
// mechanism is a narrow interface here rather than a direct Wails
// dependency. This package supplies a no-Wails default on darwin
// (os_source_darwin.go); a live, Wails ThemeChanged-backed implementation
// for every platform is kit/wailsbridge's job (WP-31) — it satisfies this
// same interface and is passed in via WithSource, nothing in this package
// needs to change to accept it.
type Source interface {
	// IsDark reports whether the OS is currently in dark mode. Called
	// live wherever Resolved() needs it — implementations that cache
	// (like the darwin default) document their own staleness, if any.
	IsDark() bool

	// Subscribe registers fn to be called whenever the OS theme changes,
	// and returns a cancel func that stops delivery. Implementations with
	// no way to detect a live change (the darwin default source has no
	// event loop to listen on outside a real Wails app) return a working,
	// no-op cancel and simply never call fn — that is the honest behavior
	// for a static source, not an error condition.
	Subscribe(fn func(dark bool)) (cancel func())
}
