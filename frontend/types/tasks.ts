// Task types — mirrors tasks/tasks.go
// Kept in sync via Go reflection test in frontend/types_test.go
//
// The snapshot shape is a contract: fields are only ever added.

import type { ErrorCode } from "./errors";

export type TaskState =
  | "queued"
  | "running"
  | "waiting"
  | "paused"
  | "stalled"
  | "done"
  | "failed"
  | "cancelled"
  | "stopped";

/** auto retries by itself; manual waits for the person; unsafe may or may not have happened. */
export type TaskRetry = "auto" | "manual" | "unsafe";

/** How a task can be stopped, as wkTask's `stop` option; false when it can't. */
export type TaskStop = "cancel" | "stop" | "stop-asks" | "pause" | "unsafe" | false;

export interface TaskError {
  /** One of the kit's codes, or an app's own. */
  code: ErrorCode | (string & {});
  /** The person's words, from errors.RegisterMessages. */
  userMsg: string;
  /** The raw error, for an ErrorBox. */
  message: string;
}

/** One task in a tasks:changed snapshot. Times are Unix milliseconds. */
export interface TaskSnapshot {
  id: string;
  /** The task this is a part of; "" for a top-level task. */
  parent: string;
  /** What it acts on, for finding its row; app-defined. */
  subject: string;
  /** What sort of work; app-defined. */
  kind: string;
  state: TaskState;
  /** Units finished. */
  done: number;
  /** 0 until the work reports a total. */
  total: number;
  unit: string;
  /** 1-based; 0 when the work has no steps. */
  step: number;
  steps: number;
  stage: string;
  /** The app's words for the current state. */
  words: string;
  stop: TaskStop;
  /** Set while failed, or while waiting for an automatic retry. */
  error: TaskError | null;
  retry: TaskRetry;
  /** When the next automatic retry runs. */
  nextTry: number | null;
  /** When it first ran; null while it has never run. */
  startedAt: number | null;
  /** The last change of any kind. */
  updatedAt: number;
}
