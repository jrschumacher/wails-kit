// Undo types — mirrors shortcuts/undo.go
// Kept in sync via Go reflection test in frontend/types_test.go
//
// The state's shape is a contract: fields are only ever added.

import type { ErrorCode } from "./errors";

/** The action at the top of the undo or redo stack. */
export interface UndoEntry {
  id: string;
  /** The action's name: "Archive". */
  name: string;
  /** The menu's words for it: "Undo Archive", "Redo Archive". */
  label: string;
}

export type UndoOp = "undo" | "redo";

export interface UndoError {
  /** One of the kit's codes, or an app's own. */
  code: ErrorCode | (string & {});
  /** The person's words, from errors.RegisterMessages. */
  userMsg: string;
  /** The raw error. */
  message: string;
}
