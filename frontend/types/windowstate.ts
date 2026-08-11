// Window state types — mirrors windowstate/windowstate.go.
// Kept in sync via Go reflection test in frontend/types_test.go.
//
// Note: windowstate has no Wails Binding (no webview-callable methods) as of
// this writing — Manager.Restore/Close are Go-side only, called around
// window creation. Only the event payloads below cross the bridge, when an
// app wires windowstate's emitter into the Wails event backend.

export interface Geometry {
  x: number;
  y: number;
  w: number;
  h: number;
  maximised: boolean;
  screenId: string;
}

// windowstate:restored event
export const WindowstateRestored = "windowstate:restored" as const;

export interface WindowstateRestoredPayload {
  name: string;
  geometry: Geometry;
  /** true if the saved geometry didn't fit an attached display. */
  clamped: boolean;
}

// windowstate:error event
export const WindowstateError = "windowstate:error" as const;

export interface WindowstateErrorPayload {
  name: string;
  /** "save" or "load" */
  op: string;
  error: string;
}
