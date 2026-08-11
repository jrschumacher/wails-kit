// Event constants and payload types — mirrors events/events.go and every kit
// package's own Event* constant + *Payload struct.
// Kept in sync via Go reflection/marshal tests in frontend/types_test.go.

import type { ErrorCode } from "./errors.js";
import {
  HealthChanged,
  type HealthChangedPayload,
} from "./health.js";
import {
  AppearanceChanged,
  type AppearanceChangedPayload,
} from "./appearance.js";
import { I18nChanged, type I18nChangedPayload } from "./i18n.js";
import {
  PermissionsChanged,
  type PermissionsChangedPayload,
} from "./permissions.js";
import {
  FirstrunTransition,
  type FirstrunTransitionPayload,
} from "./firstrun.js";
import {
  WindowstateRestored,
  type WindowstateRestoredPayload,
  WindowstateError,
  type WindowstateErrorPayload,
} from "./windowstate.js";
import {
  DiagnosticsBundleCreated,
  type DiagnosticsBundleCreatedPayload,
  DiagnosticsBundleSubmitted,
  type DiagnosticsBundleSubmittedPayload,
} from "./diagnostics.js";

export {
  HealthChanged,
  type HealthChangedPayload,
} from "./health.js";
export {
  AppearanceChanged,
  type AppearanceChangedPayload,
} from "./appearance.js";
export { I18nChanged, type I18nChangedPayload } from "./i18n.js";
export {
  PermissionsChanged,
  type PermissionsChangedPayload,
} from "./permissions.js";
export {
  FirstrunTransition,
  type FirstrunTransitionPayload,
} from "./firstrun.js";
export {
  WindowstateRestored,
  type WindowstateRestoredPayload,
  WindowstateError,
  type WindowstateErrorPayload,
} from "./windowstate.js";
export {
  DiagnosticsBundleCreated,
  type DiagnosticsBundleCreatedPayload,
  DiagnosticsBundleSubmitted,
  type DiagnosticsBundleSubmittedPayload,
} from "./diagnostics.js";

// Settings events
export const SettingsChanged = "settings:changed" as const;

export interface SettingsChangedPayload {
  keys: string[];
}

// Update events
export const UpdateAvailable = "updates:available" as const;
export const UpdateDownloading = "updates:downloading" as const;
export const UpdateReady = "updates:ready" as const;
export const UpdateError = "updates:error" as const;
export const UpdateManaged = "updates:managed" as const;

export interface UpdateAvailablePayload {
  version: string;
  releaseNotes: string;
  releaseUrl: string;
}

export interface UpdateDownloadingPayload {
  version: string;
  progress: number;
  downloaded: number;
  total: number;
}

export interface UpdateReadyPayload {
  version: string;
}

export interface UpdateErrorPayload {
  message: string;
  code: ErrorCode;
}

/** How the app was installed — see updates.InstallMethod (updates/managed.go). */
export type InstallMethod = "" | "homebrew";

export interface UpdateManagedPayload {
  method: InstallMethod;
  instructions: string;
}

// Event map for type-safe event subscription
export interface EventMap {
  [SettingsChanged]: SettingsChangedPayload;
  [UpdateAvailable]: UpdateAvailablePayload;
  [UpdateDownloading]: UpdateDownloadingPayload;
  [UpdateReady]: UpdateReadyPayload;
  [UpdateError]: UpdateErrorPayload;
  [UpdateManaged]: UpdateManagedPayload;
  [HealthChanged]: HealthChangedPayload;
  [AppearanceChanged]: AppearanceChangedPayload;
  [I18nChanged]: I18nChangedPayload;
  [PermissionsChanged]: PermissionsChangedPayload;
  [FirstrunTransition]: FirstrunTransitionPayload;
  [WindowstateRestored]: WindowstateRestoredPayload;
  [WindowstateError]: WindowstateErrorPayload;
  [DiagnosticsBundleCreated]: DiagnosticsBundleCreatedPayload;
  [DiagnosticsBundleSubmitted]: DiagnosticsBundleSubmittedPayload;
}
