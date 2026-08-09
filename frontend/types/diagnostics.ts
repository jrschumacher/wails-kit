// Diagnostics types — mirrors diagnostics/diagnostics.go and diagnostics/consent.go.
// Kept in sync via Go reflection test in frontend/types_test.go.
//
// Diagnostics has no Wails Binding split (no dedicated frontend-safe struct)
// as of this writing. Submission consent is exposed the same way any other
// settings-backed toggle is: through the generic settings.Binding
// (ResolvedSchema / GetValues / SetValues in settings.ts) via
// diagnostics.SettingsGroup()'s "diagnostics.submission_consent" key — no
// dedicated consent type is needed here. The types below cover what
// otherwise crosses the bridge: the bundle-lifecycle events (fired via the
// Service's optional emitter) and SystemInfo (GetSystemInfo(), typically
// shown on an "About" screen).

export interface SystemInfo {
  os: string;
  arch: string;
  goVersion: string;
  appName: string;
  appVersion: string;
  numCPU: number;
  /** RFC3339 timestamp string (Go time.Time's default JSON encoding). */
  timestamp: string;
}

// diagnostics:bundle_created event
export const DiagnosticsBundleCreated = "diagnostics:bundle_created" as const;

export interface DiagnosticsBundleCreatedPayload {
  path: string;
  size: number;
}

// diagnostics:bundle_submitted event
export const DiagnosticsBundleSubmitted = "diagnostics:bundle_submitted" as const;

export interface DiagnosticsBundleSubmittedPayload {
  path: string;
  statusCode: number;
}
