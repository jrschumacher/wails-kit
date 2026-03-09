// Error types — mirrors errors/errors.go and updates/service.go
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
  | "update_apply";

export interface UserError {
  code: ErrorCode;
  message: string;
  userMsg: string;
  fields?: Record<string, unknown>;
}
