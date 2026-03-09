import type { Field, SelectOption } from "@wails-kit/types";

/**
 * Resolve the active set of options for a select field.
 * If the field has dynamic options, resolves based on the controlling field's value.
 * Returns the static options if no dynamic options are defined.
 * Returns an empty array if no options apply.
 */
export function resolveOptions(
  field: Field,
  values: Record<string, unknown>,
): SelectOption[] {
  if (field.dynamicOptions) {
    const dependsOn = String(values[field.dynamicOptions.dependsOn] ?? "");
    return field.dynamicOptions.options[dependsOn] ?? [];
  }
  return field.options ?? [];
}
