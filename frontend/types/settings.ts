// Settings schema types — the wire shape returned by settings.Binding.GetSchema
// / settings.Service.GetSchema (settings/schema.go's ResolvedSchema and
// friends: ResolvedGroup, ResolvedField, ResolvedSelectOption,
// ResolvedDynamicOptions). The interface names below (Schema, Group, Field,
// SelectOption, DynamicOptions) predate the WP-12 i18n pass, when the
// authoring-time Go types (settings.Schema/Group/Field/SelectOption) were
// still what crossed the wire directly. WP-12 moved Label/Description/
// Placeholder to i18n.Text on those authoring types and introduced a
// separate Resolved* family that GetSchema actually returns — but the
// JSON tags on Resolved* were deliberately kept byte-identical to the
// pre-WP-12 wire format, so nothing below needed to change. Condition and
// Validation carry no translatable text, so both the authoring and Resolved
// Go types reuse the same struct — one TS interface each is correct.
// Kept in sync via Go reflection test in frontend/types_test.go, which
// checks these interfaces against settings.Resolved* (not settings.Schema/
// Group/Field/SelectOption/DynamicOptions, which no longer carry JSON tags
// at all since WP-12 — see that test file's comments).

export type FieldType = "text" | "password" | "select" | "toggle" | "computed" | "number";

export interface SelectOption {
  label: string;
  value: string;
}

export interface DynamicOptions {
  dependsOn: string;
  options: Record<string, SelectOption[]>;
}

export interface Condition {
  field: string;
  equals: string[];
}

export interface Validation {
  required?: boolean;
  pattern?: string;
  minLen?: number;
  maxLen?: number;
  min?: number;
  max?: number;
}

export interface Field {
  key: string;
  type: FieldType;
  label: string;
  description?: string;
  placeholder?: string;
  default?: unknown;
  options?: SelectOption[];
  dynamicOptions?: DynamicOptions;
  condition?: Condition;
  validation?: Validation;
  advanced?: boolean;
}

export interface Group {
  key: string;
  label: string;
  fields: Field[];
}

export interface Schema {
  groups: Group[];
}
