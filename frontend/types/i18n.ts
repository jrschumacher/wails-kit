// i18n types — mirrors i18n/localizer.go, i18n/catalog.go and i18n/binding.go.
// Kept in sync via Go reflection test in frontend/types_test.go.
//
// The catalog format matches i18n.Localizer.Catalog(): each entry is either
// a plain resolved string, or a plural object keyed by CLDR category name
// ("zero" | "one" | "two" | "few" | "many" | "other", "other" always
// present) for @wails-kit/i18n to resolve client-side with Intl.PluralRules.
export interface CatalogPluralEntry {
  zero?: string;
  one?: string;
  two?: string;
  few?: string;
  many?: string;
  other: string;
}

export type CatalogEntry = string | CatalogPluralEntry;

/** The shape returned by i18n.Binding.GetCatalog (Localizer.Catalog()). */
export type Catalog = Record<string, CatalogEntry>;

// i18n:changed event
export const I18nChanged = "i18n:changed" as const;

export interface I18nChangedPayload {
  locale: string;
}
