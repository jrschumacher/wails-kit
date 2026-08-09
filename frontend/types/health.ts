// Health registry types — mirrors health/health.go and health/binding.go.
// Kept in sync via Go reflection/marshal tests in frontend/types_test.go.
//
export type HealthClass = "connectivity" | "backend" | "provider";

export type HealthState = "unknown" | "healthy" | "degraded" | "down";

export interface CheckStatus {
  name: string;
  class: HealthClass;
  state: HealthState;
  critical: boolean;
  /** Omitted when the check has no error. */
  err?: string;
  /** RFC3339 timestamp string (Go time.Time's default JSON encoding). */
  checkedAt: string;
  /** Nanoseconds (Go time.Duration's default JSON encoding — an int64). */
  latency: number;
}

export interface Snapshot {
  overall: HealthState;
  offline: boolean;
  checks: CheckStatus[];
}

// health:changed event
export const HealthChanged = "health:changed" as const;

export interface HealthChangedPayload {
  check: CheckStatus;
  overall: HealthState;
}
