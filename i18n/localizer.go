package i18n

import (
	"fmt"
	"io/fs"
	"sort"
	"sync"

	"golang.org/x/text/language"

	"github.com/jrschumacher/wails-kit/v2/events"
)

// EventChanged is emitted by SetLocale whenever the resolved locale
// actually changes (transition-only, like the kit's other *:changed
// events).
const EventChanged = "i18n:changed"

// ChangedPayload is EventChanged's payload.
type ChangedPayload struct {
	Locale string `json:"locale"`
}

// Localizer resolves Text values against merged locale catalogs. Build one
// with New; it is safe for concurrent use.
type Localizer struct {
	mu sync.RWMutex

	locale language.Tag

	catalogs map[string]map[string]catalogEntry // locale tag string -> key -> entry
	matcher  language.Matcher

	emitter *events.Emitter

	// resolution inputs, retained so SetLocale can re-run the same matching
	// logic New used for the explicit tier.
	explicit      string
	settingsSrc   SettingsSource
	defaultTagRaw string
	defaultTag    language.Tag

	pendingCatalogs []fs.FS // staged by WithCatalog, merged in New after options run
}

// New builds a Localizer: merges catalogs (kit's own first, then each
// WithCatalog source in order), then resolves the initial locale once via
// the full order — explicit > settings > env > OS source > default (OQ-5)
// — and caches it. Nothing in a Localizer re-execs the OS source or
// re-reads settings after this; SetLocale is the only way to change locale
// afterward.
func New(opts ...Option) (*Localizer, error) {
	l := &Localizer{
		catalogs: make(map[string]map[string]catalogEntry),
	}

	if err := l.mergeFS(kitLocalesFS); err != nil {
		return nil, fmt.Errorf("i18n: embedded kit catalog: %w", err)
	}

	for _, opt := range opts {
		opt(l)
	}

	for _, src := range l.pendingCatalogs {
		if err := l.mergeFS(src); err != nil {
			return nil, fmt.Errorf("i18n: catalog: %w", err)
		}
	}
	l.pendingCatalogs = nil

	l.defaultTag = language.English
	if l.defaultTagRaw != "" {
		if tag, err := language.Parse(l.defaultTagRaw); err == nil {
			l.defaultTag = tag
		}
	}

	l.matcher = language.NewMatcher(l.supportedTags())
	l.locale = l.resolveInitial()

	return l, nil
}

// supportedTags returns the locales present in the merged catalog plus the
// configured default, default first (language.NewMatcher uses its first
// element as the no-match fallback), in a stable order.
func (l *Localizer) supportedTags() []language.Tag {
	tags := []language.Tag{l.defaultTag}
	seen := map[string]bool{l.defaultTag.String(): true}

	keys := make([]string, 0, len(l.catalogs))
	for k := range l.catalogs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if seen[k] {
			continue
		}
		if tag, err := language.Parse(k); err == nil {
			tags = append(tags, tag)
			seen[k] = true
		}
	}
	return tags
}

// Locale returns the currently resolved locale as a BCP-47 tag string.
func (l *Localizer) Locale() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.locale.String()
}

// SetLocale explicitly changes the locale, taking the same precedence an
// explicit WithLocale would have had at construction. tag must parse as a
// BCP-47 language tag; it need not have a catalog of its own — lookups fall
// back through language.Tag.Parent() and finally the configured default
// (see lookupCandidates). Emits EventChanged, but only when the resolved
// locale actually changes.
func (l *Localizer) SetLocale(tag string) error {
	parsed, err := language.Parse(tag)
	if err != nil {
		return fmt.Errorf("i18n: invalid locale %q: %w", tag, err)
	}

	l.mu.Lock()
	changed := l.locale != parsed
	l.locale = parsed
	l.explicit = tag
	l.mu.Unlock()

	if changed && l.emitter != nil {
		l.emitter.Emit(EventChanged, ChangedPayload{Locale: parsed.String()})
	}
	return nil
}

// lookup finds the catalog entry for key, walking lookupCandidates for the
// currently resolved locale. Returns nil if no catalog (at any fallback
// tier) defines key.
func (l *Localizer) lookup(key string) *catalogEntry {
	l.mu.RLock()
	tag, def, catalogs := l.locale, l.defaultTag, l.catalogs
	l.mu.RUnlock()

	for _, candidate := range lookupCandidates(tag, def) {
		m, ok := catalogs[candidate]
		if !ok {
			continue
		}
		if entry, ok := m[key]; ok {
			return &entry
		}
	}
	return nil
}

// T resolves t against the current locale's catalog (falling back through
// ancestor locales, the configured default, and finally t.Other — see
// lookup / lookupCandidates). If args are given, the resolved format string
// is passed through fmt.Sprintf; with no args, it is returned as-is (so a
// message containing a literal "%" is never misinterpreted as a broken
// verb).
func (l *Localizer) T(t Text, args ...any) string {
	format := t.Other
	if entry := l.lookup(t.Key); entry != nil {
		format = entry.Other
	}
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// Catalog returns the merged, locale-resolved catalog for frontend
// hydration: least-specific to most-specific ancestor of the current locale
// applied in order (so the current locale's own strings win), falling back
// to the configured default for anything the current locale doesn't
// override. Plural entries are returned as map[string]string keyed by CLDR
// category ("one", "other", ...) — exactly the catalog JSON shape — so
// @wails-kit/i18n can resolve them client-side with Intl.PluralRules.
func (l *Localizer) Catalog() map[string]any {
	l.mu.RLock()
	tag, def, catalogs := l.locale, l.defaultTag, l.catalogs
	l.mu.RUnlock()

	candidates := lookupCandidates(tag, def)

	result := make(map[string]any)
	for i := len(candidates) - 1; i >= 0; i-- {
		m, ok := catalogs[candidates[i]]
		if !ok {
			continue
		}
		for key, entry := range m {
			if entry.Forms != nil {
				forms := make(map[string]string, len(entry.Forms))
				for form, text := range entry.Forms {
					forms[pluralFormNames[form]] = text
				}
				result[key] = forms
			} else {
				result[key] = entry.Other
			}
		}
	}
	return result
}
