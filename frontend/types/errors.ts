// Error types — mirrors errors/errors.go, updates/service.go and settings/check.go
// Kept in sync via Go reflection test in frontend/types_test.go

export type ErrorCode =
  // Core error codes (errors package)
  | "auth_invalid"
  | "auth_expired"
  | "auth_missing"
  | "not_found"
  | "permission_denied"
  | "validation"
  | "rate_limited"
  | "timeout"
  | "cancelled"
  | "internal"
  | "storage_read"
  | "storage_write"
  | "config_invalid"
  | "config_missing"
  | "provider_error"
  // Update error codes (updates package)
  | "update_check"
  | "update_download"
  | "update_apply"
  | "update_verify"
  // Settings check error codes (settings package)
  | "settings_check_failed"
  | "settings_check_timeout"
  | "settings_unknown_setting";

export interface UserError {
  code: ErrorCode;
  message: string;
  userMsg: string;
  fields?: Record<string, unknown>;
}
