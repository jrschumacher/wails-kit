import type { Catalog, CatalogEntry } from "@wails-kit/types";
import { format, type Text } from "./text.js";

/**
 * resolveText resolves txt against catalog: the catalog entry's string (or,
 * for a plural entry, its "other" form) if present, otherwise txt.other.
 * Mirrors Localizer.T's fallback behavior (i18n/localizer.go) for the
 * plain-string case — args are applied fmt.Sprintf-style via format().
 */
export function resolveText(
  catalog: Catalog,
  txt: Text,
  ...args: unknown[]
): string {
  const template = entryToTemplate(catalog[txt.key]) ?? txt.other;
  return format(template, args);
}

/**
 * resolvePlural resolves txt against catalog using CLDR plural rules for
 * locale to pick the category (Localizer.TN's client-side counterpart —
 * i18n/localizer.go's TN does the equivalent server-side with
 * golang.org/x/text/feature/plural). n is passed through to format() as
 * the first substitution argument, mirroring how Go call sites pass n as
 * TN's first arg (e.g. `l.TN(msg, n, n)` for "%d items").
 *
 * If the catalog has no entry for txt.key, or the entry is a plain string
 * rather than a plural object, this falls back to txt.other (or the plain
 * string) the same way resolveText does — a missing/non-plural catalog
 * entry is not an error here, matching the kit's "no localizer/catalog,
 * still sensible English" guarantee (see i18n/AGENTS.md, errors/AGENTS.md).
 */
export function resolvePlural(
  catalog: Catalog,
  txt: Text,
  n: number,
  locale: string,
  ...args: unknown[]
): string {
  const entry = catalog[txt.key];
  let template: string;
  if (entry && typeof entry === "object") {
    const category = selectPluralCategory(entry, n, locale);
    template = entry[category] ?? entry.other ?? txt.other;
  } else {
    template = entryToTemplate(entry) ?? txt.other;
  }
  return format(template, [n, ...args]);
}

/** entryToTemplate reduces a CatalogEntry to its single format-string form. */
function entryToTemplate(entry: CatalogEntry | undefined): string | undefined {
  if (entry == null) {
    return undefined;
  }
  return typeof entry === "string" ? entry : entry.other;
}

/**
 * selectPluralCategory picks the CLDR plural category for n under locale
 * via Intl.PluralRules, falling back to "other" when the catalog entry
 * doesn't carry that category (every entry is guaranteed to carry "other"
 * — see i18n.CatalogPluralEntry) or when locale fails to construct an
 * Intl.PluralRules (an invalid/unsupported BCP-47 tag).
 */
function selectPluralCategory(
  entry: Exclude<CatalogEntry, string>,
  n: number,
  locale: string,
): "zero" | "one" | "two" | "few" | "many" | "other" {
  try {
    const category = new Intl.PluralRules(locale).select(n) as
      | "zero"
      | "one"
      | "two"
      | "few"
      | "many"
      | "other";
    return entry[category] != null ? category : "other";
  } catch {
    return "other";
  }
}
