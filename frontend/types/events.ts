// Event constants and payload types — mirrors events/events.go and updates/service.go
// Kept in sync via Go reflection test in frontend/types_test.go

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
  code: string;
}

// Event map for type-safe event subscription
export interface EventMap {
  [SettingsChanged]: SettingsChangedPayload;
  [UpdateAvailable]: UpdateAvailablePayload;
  [UpdateDownloading]: UpdateDownloadingPayload;
  [UpdateReady]: UpdateReadyPayload;
  [UpdateError]: UpdateErrorPayload;
}
