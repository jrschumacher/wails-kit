// Event constants and payload types — mirrors events/events.go, updates/service.go, tasks/tasks.go, desktop/desktop.go and shortcuts/undo.go
// Kept in sync via Go reflection test in frontend/types_test.go

import type { ErrorCode } from "./errors";
import type { TaskSnapshot } from "./tasks";
import type { UndoEntry, UndoError, UndoOp } from "./undo";

// Settings events
export const SettingsChanged = "settings:changed" as const;

export interface SettingsChangedPayload {
  keys: string[];
}

// Settings check events (settings/check.go)
export const SettingsCheckFailed = "settings:check_failed" as const;

export interface SettingsCheckFailedPayload {
  key: string;
  label: string;
  message: string;
  notice: string;
  attention: boolean;
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
  code: ErrorCode;
}

// Task events
export const TasksChanged = "tasks:changed" as const;

/** Every task in the list (active ones, and finished ones not yet dismissed), in the order they were added. */
export interface TasksChangedPayload {
  tasks: TaskSnapshot[];
}

// Desktop events
export const DesktopShowTasks = "desktop:show-tasks" as const;
export const DesktopQuitPending = "desktop:quit-pending" as const;

/** The top-level tasks a clicked notification was about: select their results. */
export interface DesktopShowTasksPayload {
  tasks: string[];
}

/** Whether the app quits once the work finishes (the person chose Quit When Done). */
export interface DesktopQuitPendingPayload {
  pending: boolean;
  running: number;
}

// Undo events (shortcuts/undo.go)
export const UndoChanged = "undo:changed" as const;
export const UndoText = "undo:text" as const;
export const UndoFailed = "undo:failed" as const;

/** The app's undo stack after a change. */
export interface UndoChangedPayload {
  undo: UndoEntry | null;
  redo: UndoEntry | null;
  /** An undo or redo is running. */
  busy: boolean;
  /** A text field has focus: Edit > Undo undoes typing first. */
  textFocus: boolean;
}

/** Edit > Undo or Redo while a text field has focus: undo typing in the focused window. */
export interface UndoTextPayload {
  op: UndoOp;
}

/** An undo or redo started from the menu failed. */
export interface UndoFailedPayload {
  op: UndoOp;
  id: string;
  name: string;
  error: UndoError;
}

// Event map for type-safe event subscription
export interface EventMap {
  [SettingsChanged]: SettingsChangedPayload;
  [SettingsCheckFailed]: SettingsCheckFailedPayload;
  [UpdateAvailable]: UpdateAvailablePayload;
  [UpdateDownloading]: UpdateDownloadingPayload;
  [UpdateReady]: UpdateReadyPayload;
  [UpdateError]: UpdateErrorPayload;
  [TasksChanged]: TasksChangedPayload;
  [DesktopShowTasks]: DesktopShowTasksPayload;
  [DesktopQuitPending]: DesktopQuitPendingPayload;
  [UndoChanged]: UndoChangedPayload;
  [UndoText]: UndoTextPayload;
  [UndoFailed]: UndoFailedPayload;
}
