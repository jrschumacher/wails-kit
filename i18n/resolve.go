package i18n

import (
	"os"
	"strings"

	"golang.org/x/text/language"
)

// envLocaleVars is the POSIX locale environment precedence: LC_ALL beats
// LC_MESSAGES beats LANG. This is tier 3 of the resolution order (OQ-5).
var envLocaleVars = []string{"LC_ALL", "LC_MESSAGES", "LANG"}

// resolveInitial runs the full resolution order (OQ-5, decided; highest
// wins) once, at construction:
//
//  1. explicit WithLocale option
//  2. settings override, if a settings service is wired (WithSettings)
//  3. LC_ALL / LC_MESSAGES / LANG
//  4. OS source (osLocales — defaults(1) AppleLanguages on darwin, nil
//     elsewhere; see os_locale_*.go)
//  5. configured default (WithDefault, else English)
//
// Tiers 1-3 carry a single candidate tag and are accepted as soon as they
// parse as valid BCP-47 — Locale() should report what was actually
// requested/detected even if no catalog exists for it yet (that gap is
// handled separately, as a per-key/per-lookup fallback in lookup — see
// localizer.go). Tier 4 is different: the OS source hands back an *ordered
// preference list*, which is precisely what language.Matcher is for (OQ-5:
// "AppleLanguages returns the user's ordered language list — that is what
// language.Matcher wants"), so it is matched against the supported
// (catalog+default) tags to pick the best-supported one.
func (l *Localizer) resolveInitial() language.Tag {
	if l.explicit != "" {
		if tag, err := language.Parse(l.explicit); err == nil {
			return tag
		}
	}

	if l.settingsSrc != nil {
		if raw := settingsLocale(l.settingsSrc); raw != "" && raw != "system" {
			if tag, err := language.Parse(raw); err == nil {
				return tag
			}
		}
	}

	for _, name := range envLocaleVars {
		if v, ok := os.LookupEnv(name); ok {
			if tag, ok := parseEnvLocale(v); ok {
				return tag
			}
		}
	}

	if raw := osLocales(); len(raw) > 0 {
		want := make([]language.Tag, 0, len(raw))
		for _, s := range raw {
			if tag, err := language.Parse(s); err == nil {
				want = append(want, tag)
			}
		}
		if len(want) > 0 {
			tag, _, _ := l.matcher.Match(want...)
			return tag
		}
	}

	return l.defaultTag
}

// settingsLocale reads the persisted locale override from a SettingsSource,
// tolerating any error (missing/unreadable settings just means "no
// override") or a non-string value.
func settingsLocale(src SettingsSource) string {
	values, err := src.GetValues()
	if err != nil {
		return ""
	}
	raw, _ := values[SettingLocale].(string)
	return raw
}

// parseEnvLocale parses a POSIX locale environment value such as
// "en_US.UTF-8" or "fr_CA.UTF-8@euro" into a BCP-47 tag. "C" and "POSIX"
// (the POSIX "no locale" sentinels) and empty values report no match, as
// does anything that fails to parse as a language tag.
func parseEnvLocale(v string) (language.Tag, bool) {
	v = strings.TrimSpace(v)
	if v == "" || v == "C" || v == "POSIX" {
		return language.Tag{}, false
	}
	if i := strings.IndexAny(v, ".@"); i >= 0 {
		v = v[:i]
	}
	v = strings.ReplaceAll(v, "_", "-")
	tag, err := language.Parse(v)
	if err != nil {
		return language.Tag{}, false
	}
	return tag, true
}

// lookupCandidates returns the ordered chain of locale-tag strings to try
// when resolving a catalog key for tag: tag itself, then each ancestor
// (language.Tag.Parent — e.g. "fr-CA" -> "fr"), then the configured default.
// This is what makes missing-locale lookups degrade gracefully instead of
// going straight to Text.Other: a resolved locale with no catalog of its own
// still finds strings via a shared ancestor or the default's catalog.
func lookupCandidates(tag, def language.Tag) []string {
	out := make([]string, 0, 4)
	seen := make(map[string]bool, 4)

	add := func(t language.Tag) {
		s := t.String()
		if s == "" || s == "und" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}

	t := tag
	for range 5 { // BCP-47 tags don't nest deeper than this in practice
		add(t)
		parent := t.Parent()
		if parent == t {
			break
		}
		t = parent
	}
	add(def)
	return out
}
