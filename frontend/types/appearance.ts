// Appearance types — mirrors appearance/appearance.go and appearance/binding.go.
// Kept in sync via Go reflection test in frontend/types_test.go.

/** User preference: follow the OS, or pin to a theme. */
export type AppearanceMode = "system" | "light" | "dark";

/** Resolved truth: exactly two states, never "system". */
export type Theme = "light" | "dark";

// appearance:changed event
export const AppearanceChanged = "appearance:changed" as const;

export interface AppearanceChangedPayload {
  mode: AppearanceMode;
  resolved: Theme;
}
