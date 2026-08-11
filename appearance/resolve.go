package appearance

// resolveTheme is the pure resolution rule: mode != ModeSystem wins outright
// (the user's explicit override); otherwise the Source's live signal
// decides, and a nil Source resolves to ThemeLight (documented default —
// see Source's doc comment and the README).
func resolveTheme(mode Mode, source Source) Theme {
	switch mode {
	case ModeLight:
		return ThemeLight
	case ModeDark:
		return ThemeDark
	}
	if source == nil {
		return ThemeLight
	}
	if source.IsDark() {
		return ThemeDark
	}
	return ThemeLight
}

// currentMode returns the effective mode: read live from the wired
// settings.Service if there is one (values are read at call time, not
// cached — matching every other kit package's settings integration, see
// docs/settings-integration.md), falling back to the in-memory value
// (s.mode, updated by SetMode) when there's no settings.Service, the read
// fails, or the stored value isn't a recognized Mode.
func (s *Service) currentMode() Mode {
	s.mu.Lock()
	settingsSvc := s.settingsSvc
	fallback := s.mode
	s.mu.Unlock()

	if settingsSvc == nil {
		return fallback
	}

	values, err := settingsSvc.GetValues()
	if err != nil {
		return fallback
	}
	raw, ok := values[SettingMode].(string)
	if !ok {
		return fallback
	}
	m := Mode(raw)
	if !validMode(m) {
		return fallback
	}
	return m
}

// resolveAndMaybeEmit resolves mode against s.source, and emits
// EventChanged (with the lock released — see the package's "never emit
// while holding a lock" rule) if and only if the resolved Theme differs
// from the last known value. Used by SetMode, Refresh, and onSourceChange —
// the three ways a resolved-Theme change can originate.
func (s *Service) resolveAndMaybeEmit(mode Mode) Theme {
	resolved := resolveTheme(mode, s.source)

	s.mu.Lock()
	changed := resolved != s.lastResolved
	s.lastResolved = resolved
	s.mu.Unlock()

	if changed {
		s.emitChanged(mode, resolved)
	}
	return resolved
}

// onSourceChange is the callback passed to Source.Subscribe. An OS flip
// only affects the resolved Theme while the effective mode is ModeSystem —
// an explicit override in effect means the OS signal is irrelevant to what
// Resolved() reports, so this is a deliberate no-op in that case, not a
// missed update.
func (s *Service) onSourceChange(_ bool) {
	mode := s.currentMode()
	if mode != ModeSystem {
		return
	}
	s.resolveAndMaybeEmit(mode)
}

func (s *Service) emitChanged(mode Mode, resolved Theme) {
	if s.emitter == nil {
		return
	}
	s.emitter.Emit(EventChanged, ChangedPayload{Mode: mode, Resolved: resolved})
}
