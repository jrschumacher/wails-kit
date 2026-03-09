// Settings schema types — mirrors settings/schema.go
// Kept in sync via Go reflection test in frontend/types_test.go

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
