package i18n

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

// kitLocalesFS is the kit's own catalog. It is always merged first, so any
// app-supplied catalog (via WithCatalog) overrides it on key collision —
// AD-5: "the localizer merges kit catalogs with app catalogs (app wins on
// key collision)".
//
//go:embed locales/*.json
var kitLocalesFS embed.FS

// pluralForms maps CLDR plural category names, as they appear in catalog
// JSON, to plural.Form. Every category not present in a given entry is
// simply absent from catalogEntry.Forms; "other" is the only one required.
var pluralForms = map[string]plural.Form{
	"zero":  plural.Zero,
	"one":   plural.One,
	"two":   plural.Two,
	"few":   plural.Few,
	"many":  plural.Many,
	"other": plural.Other,
}

// pluralFormNames is the reverse of pluralForms, used when re-serializing a
// plural entry for frontend hydration (Localizer.Catalog).
var pluralFormNames = func() map[plural.Form]string {
	m := make(map[plural.Form]string, len(pluralForms))
	for name, form := range pluralForms {
		m[form] = name
	}
	return m
}()

// catalogEntry is one resolved catalog value: either a plain string, or a
// set of CLDR plural forms (always including "other").
type catalogEntry struct {
	Other string
	Forms map[plural.Form]string // nil for non-plural entries
}

// UnmarshalJSON accepts either a JSON string (a plain message) or a JSON
// object of plural-category -> string (a plural message), per the catalog
// format documented in the package README:
//
//	"key.one":  "a plain string"
//	"key.two":  {"one": "%d item", "other": "%d items"}
func (e *catalogEntry) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*e = catalogEntry{Other: s}
		return nil
	}

	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("i18n: catalog value must be a string or a plural object: %w", err)
	}
	forms := make(map[plural.Form]string, len(raw))
	for name, text := range raw {
		form, ok := pluralForms[name]
		if !ok {
			return fmt.Errorf("i18n: unknown plural category %q", name)
		}
		forms[form] = text
	}
	other, ok := forms[plural.Other]
	if !ok {
		return fmt.Errorf(`i18n: plural object missing required "other" form`)
	}
	*e = catalogEntry{Other: other, Forms: forms}
	return nil
}

// mergeFS loads every locales/<bcp47>.json file found in fsys and merges its
// entries into l.catalogs, keyed by the *canonical* BCP-47 form of the
// locale tag in the filename — not the raw filename stem. "fr-CA.json"
// becomes locale "fr-CA", but so does "fr_ca.json" or "FR-CA.json": the
// filename stem is run through language.Parse and re-serialized via
// Tag.String(), the same canonicalization lookupCandidates (resolve.go)
// produces from a resolved locale tag. Without this, a catalog keyed by
// whatever casing/separator/legacy-code convention a translator happened to
// name the file with (e.g. "en-us.json", "pt_BR.json", "iw.json" — the
// deprecated code for Hebrew) never matches lookupCandidates' canonical
// candidates ("en-US", "pt-BR", "he"): the file loads without error, its
// locale even appears in LocaleOptions, but every T/TN/Catalog lookup for it
// silently falls through to a lower fallback tier instead. A filename stem
// that fails to parse as BCP-47 at all is kept as-is (rather than dropping
// the file) — see the fallback assignment below.
//
// A missing "locales" directory is not an error — a WithCatalog source may
// legitimately ship none. Entries from later mergeFS calls overwrite entries
// from earlier ones for the same locale+key, which is what makes app
// catalogs win over the kit's.
func (l *Localizer) mergeFS(fsys fs.FS) error {
	entries, err := fs.ReadDir(fsys, "locales")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		locale := strings.TrimSuffix(entry.Name(), ".json")
		if tag, err := language.Parse(locale); err == nil {
			locale = tag.String()
		}

		data, err := fs.ReadFile(fsys, path.Join("locales", entry.Name()))
		if err != nil {
			return fmt.Errorf("i18n: read %s: %w", entry.Name(), err)
		}

		var raw map[string]catalogEntry
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("i18n: parse %s: %w", entry.Name(), err)
		}

		m, ok := l.catalogs[locale]
		if !ok {
			m = make(map[string]catalogEntry, len(raw))
			l.catalogs[locale] = m
		}
		for key, value := range raw {
			m[key] = value
		}
	}
	return nil
}
