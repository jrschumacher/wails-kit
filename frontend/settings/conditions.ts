import type { Condition } from "@wails-kit/types";

/**
 * Evaluate whether a field's condition is met given the current values.
 * Returns true if the condition is satisfied (field should be visible).
 * Returns true if condition is undefined (unconditionally visible).
 */
export function conditionMet(
  condition: Condition | undefined,
  values: Record<string, unknown>,
): boolean {
  if (!condition) {
    return true;
  }
  const val = String(values[condition.field] ?? "");
  return condition.equals.some((eq) => val === eq);
}
