// Firstrun types — mirrors firstrun/firstrun.go.
// Kept in sync via Go reflection test in frontend/types_test.go.
//
// Note: firstrun has no Wails Binding (no webview-callable methods) as of
// this writing — Service.Run/Detect are Go-side only. Only the event
// payload below crosses the bridge, when an app wires firstrun's emitter
// into the Wails event backend.

export type FirstrunKind = "fresh" | "upgrade" | "downgrade" | "same";

// firstrun:transition event
export const FirstrunTransition = "firstrun:transition" as const;

export interface FirstrunTransitionPayload {
  kind: FirstrunKind;
  /** Empty string for a Fresh transition. */
  previous: string;
  current: string;
}
