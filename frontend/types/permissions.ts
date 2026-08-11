// Permissions types — mirrors permissions/permissions.go and permissions/binding.go.
// Kept in sync via Go reflection test in frontend/types_test.go.

export type PermissionKind = "notifications" | "accessibility" | "full_disk_access";

/**
 * Deliberately four-valued — see permissions/permissions.go's package doc
 * ("Three states, not two"). "denied" and "not_determined" demand different
 * UI: not_determined -> show rationale then Request; denied -> direct the
 * user to OpenSystemSettings, never re-prompt. "unsupported" means the Kind
 * isn't a concept on this platform/build and must never be conflated with
 * "denied".
 */
export type PermissionStatus = "granted" | "denied" | "not_determined" | "unsupported";

// permissions:changed event
export const PermissionsChanged = "permissions:changed" as const;

export interface PermissionsChangedPayload {
  kind: PermissionKind;
  status: PermissionStatus;
}
