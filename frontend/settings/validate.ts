import type { Field, Schema, SelectOption } from "@wails-kit/types";
import { conditionMet } from "./conditions";

/** Validation error codes matching settings/validate.go constants. */
export const CodeRequired = "required" as const;
export const CodePattern = "pattern" as const;
export const CodeMinLen = "min_length" as const;
export const CodeMaxLen = "max_length" as const;
export const CodeMin = "min" as const;
export const CodeMax = "max" as const;
export const CodeInvalidType = "invalid_type" as const;
export const CodeInvalidOption = "invalid_option" as const;

export type ValidationCode =
  | typeof CodeRequired
  | typeof CodePattern
  | typeof CodeMinLen
  | typeof CodeMaxLen
  | typeof CodeMin
  | typeof CodeMax
  | typeof CodeInvalidType
  | typeof CodeInvalidOption;

export interface ValidationError {
  field: string;
  message: string;
  code: ValidationCode;
}

/**
 * Validate settings values against a schema.
 * Mirrors settings.Validate in Go.
 */
export function validate(
  schema: Schema,
  values: Record<string, unknown>,
): ValidationError[] {
  const errs: ValidationError[] = [];

  for (const group of schema.groups) {
    for (const field of group.fields) {
      if (
        !field.validation &&
        field.type !== "select" &&
        field.type !== "toggle"
      ) {
        continue;
      }

      if (field.condition && !conditionMet(field.condition, values)) {
        continue;
      }

      const val = values[field.key];
      errs.push(...validateField(field, val, values));
    }
  }

  return errs;
}

function validateField(
  field: Field,
  val: unknown,
  values: Record<string, unknown>,
): ValidationError[] {
  const errs: ValidationError[] = [];
  const v = field.validation;

  const isStr = typeof val === "string";
  const str = isStr ? (val as string) : "";
  const isNum = typeof val === "number" && !Number.isNaN(val);

  if (v?.required) {
    if (val == null || (isStr && str === "")) {
      errs.push({
        field: field.key,
        message: `${field.label} is required`,
        code: CodeRequired,
      });
      return errs;
    }
  }

  // Toggle type validation: must be a bool if provided
  if (field.type === "toggle" && val != null) {
    if (typeof val !== "boolean") {
      errs.push({
        field: field.key,
        message: `${field.label} must be true or false`,
        code: CodeInvalidType,
      });
    }
  }

  if (isStr && str !== "" && v) {
    if (v.pattern) {
      const re = new RegExp(v.pattern);
      if (!re.test(str)) {
        errs.push({
          field: field.key,
          message: `${field.label} has invalid format`,
          code: CodePattern,
        });
      }
    }
    // Use spread operator to count Unicode code points (like Go's RuneCountInString)
    const runeCount = [...str].length;
    if (v.minLen && runeCount < v.minLen) {
      errs.push({
        field: field.key,
        message: `${field.label} must be at least ${v.minLen} characters`,
        code: CodeMinLen,
      });
    }
    if (v.maxLen && runeCount > v.maxLen) {
      errs.push({
        field: field.key,
        message: `${field.label} must be at most ${v.maxLen} characters`,
        code: CodeMaxLen,
      });
    }
  }

  if (isNum && v) {
    const n = val as number;
    if (v.min != null && n < v.min) {
      errs.push({
        field: field.key,
        message: `${field.label} must be at least ${v.min}`,
        code: CodeMin,
      });
    }
    if (v.max != null && n > v.max) {
      errs.push({
        field: field.key,
        message: `${field.label} must be at most ${v.max}`,
        code: CodeMax,
      });
    }
  }

  if (
    field.type === "select" &&
    isStr &&
    str !== "" &&
    hasSelectableOptions(field, values) &&
    !selectOptionAllowed(field, str, values)
  ) {
    errs.push({
      field: field.key,
      message: `${field.label} has an invalid option`,
      code: CodeInvalidOption,
    });
  }

  return errs;
}

function hasSelectableOptions(
  field: Field,
  values: Record<string, unknown>,
): boolean {
  if (field.dynamicOptions) {
    const dependsOn = String(values[field.dynamicOptions.dependsOn] ?? "");
    return (field.dynamicOptions.options[dependsOn]?.length ?? 0) > 0;
  }
  return (field.options?.length ?? 0) > 0;
}

function selectOptionAllowed(
  field: Field,
  value: string,
  values: Record<string, unknown>,
): boolean {
  let options: SelectOption[] | undefined = field.options;
  if (field.dynamicOptions) {
    const dependsOn = String(values[field.dynamicOptions.dependsOn] ?? "");
    options = field.dynamicOptions.options[dependsOn];
  }
  return options?.some((o) => o.value === value) ?? false;
}
