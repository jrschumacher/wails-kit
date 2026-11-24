// Types for @wails-kit/ui (optional: the package is plain JavaScript).
// Settings shapes mirror frontend/types settings.ts and the Go settings package.

export interface SelectOption { label: string; value: string }
export interface Condition { field: string; equals: string[] }
export interface Validation { required?: boolean; pattern?: string; minLen?: number; maxLen?: number; min?: number; max?: number }
export type FieldType = 'text' | 'password' | 'select' | 'toggle' | 'computed' | 'number';
export interface Field {
  key: string; type: FieldType; label: string; description?: string; placeholder?: string; default?: unknown;
  options?: SelectOption[]; dynamicOptions?: { dependsOn: string; options: Record<string, SelectOption[]> };
  condition?: Condition; validation?: Validation; check?: { pending?: string }; advanced?: boolean;
}
export interface Group { key: string; label: string; fields: Field[] }
export interface Schema { groups: Group[] }
export interface ValidationError { field: string; message: string; code: string }
export type Values = Record<string, unknown>;

// ---- Custom elements (kit.js) ----
export class WkMenu extends HTMLElement { readonly open: boolean; show(focus?: 'first' | 'last'): void; close(): void }
export class WkContextMenu extends HTMLElement { readonly open: boolean; close(): void }
export class WkTabs extends HTMLElement { selectedIndex: number }
export class WkTooltip extends HTMLElement { readonly open: boolean; show(): void; hide(): void }
export class WkDialog extends HTMLElement { readonly open: boolean; show(options?: { opener?: Element }): void; close(returnValue?: string): void }
export function syncSteppers(root?: ParentNode | Element): void;
export interface ProgressReport { done?: number; total: number; unit?: string; text?: string }
export function setProgress(bar: HTMLElement, progress: ProgressReport, options?: { aria?: boolean | 'now'; clock?: () => number }): void;

// ---- Timings and motion (motion.js) ----
export const timings: Readonly<{
  indicatorDelay: number; indicatorMin: number; stopDelay: number; waiting: number; timeout: number;
  estimateStable: number; loadingLine: number; recheckDelay: number; toast: number; toastWithAction: number;
  copied: number; progressValueEvery: number; backoff: readonly number[]; backoffJitter: number; wakeSettle: number;
}>;
export const motion: Readonly<{ durationFast: number; durationBase: number; durationSlow: number; easeOut: string; easeIn: string }>;
export function reducedMotion(): boolean;

// ---- Announcer (announce.js) ----
/** Says words through the window's polite (or assertive) live region; messages within 500 ms are joined. */
export function announce(words: string, options?: { assertive?: boolean }): void;
/** Moves the announcer into the top-most open modal dialog (or back to <body>); wk-dialog and the dialog toggle/close events call it. */
export function placeAnnouncer(dialog?: HTMLDialogElement | null, open?: boolean): void;

// ---- Tasks (task.js) ----
export type TaskState = 'queued' | 'running' | 'waiting' | 'paused' | 'stalled' | 'done' | 'failed' | 'cancelled' | 'stopped';
export type TaskStopMode = 'cancel' | 'stop' | 'stop-asks' | 'pause' | 'unsafe' | false;
/** A tasks:changed snapshot, as @wails-kit/types' TaskSnapshot. */
export interface TaskSnapshotLike {
  id: string; parent: string; subject: string; kind: string; state: TaskState;
  done: number; total: number; unit: string; step: number; steps: number; stage: string; words: string;
  stop: TaskStopMode; error: { code: string; userMsg: string; message: string } | null;
  retry: 'auto' | 'manual' | 'unsafe'; nextTry: number | null; startedAt: number | null; updatedAt: number;
}
export interface TaskProgress { done?: number; total?: number; unit?: string; step?: number; steps?: number; stage?: string }
export interface TaskSpec {
  /** The element (or elements) that started it: busy (aria-disabled, data-busy) until it ends. */
  control?: Element | Element[] | null;
  /** The element that shows the state (a row's state cell): a dot and the words replace its content while it runs. */
  target?: Element | null;
  /** A .wk-progress the app placed where the work shows: hidden until the indicator, then moved by progress(). */
  bar?: HTMLElement | null;
  words: string;
  subject?: string; kind?: string; id?: string;
  expect?: 'short' | 'maybe-long' | 'long';
  stop?: TaskStopMode;
  /** The app stops the work; resolve false to leave the task running (the person said no, or the app reports the end). */
  onStop?: (task: TaskHandle) => unknown;
  onPause?: (task: TaskHandle) => unknown;
  onResume?: (task: TaskHandle) => unknown;
  onTimeout?: (task: TaskHandle) => void;
  onEnd?: (detail: TaskEventDetail) => void;
  /** ms without a report before it fails; false for none. Default timings.timeout. */
  timeout?: number | false;
  timeoutWords?: string;
  /** Words for the switch to Waiting after 30 s without a report. */
  waitingWords?: string;
  /** Start Queued, with these words. */
  queued?: string;
  /** Where Esc applies and a failure is assertive: default the Sheet it is in, else the control and the target's row. */
  scope?: Element;
  /** Esc and ⌘. cancel it while focus is in scope. Default: stop is 'cancel' and expect isn't 'long'. */
  esc?: boolean;
  /** Where the Stop button goes; default after the last control, else in the target. */
  stopIn?: Element;
  /** Work in parts: the unit the summary counts ("reviewers"), and its words. */
  unit?: string;
  summary?: (counts: { finished: number; failed: number; stopped: number; cancelled: number; total: number; unit: string }) => string;
}
export interface TaskEventDetail { id: string; state: TaskState; words: string; [key: string]: unknown }
export interface TaskEnd { words: string; error?: unknown; retry?: () => void; visible?: boolean; show?: () => void; showLabel?: string; [key: string]: unknown }
export interface TaskHandle {
  readonly id: string;
  readonly state: TaskState;
  /** Aborts on Cancel, Stop, a timeout or a failure. */
  readonly signal: AbortSignal;
  progress(p: TaskProgress): void;
  setWords(words: string): void;
  queued(words?: string): void;
  running(words?: string): void;
  waiting(words?: string, options?: { timeout?: number | false }): void;
  paused(words?: string): void;
  resumed(words?: string): void;
  stalled(words: string): void;
  /** Resolve in the step the indicator goes (at least 500 ms after it showed): draw the result there. */
  done(end: TaskEnd | string): Promise<TaskEventDetail | null>;
  fail(end: TaskEnd | string): Promise<TaskEventDetail | null>;
  stopped(words?: string): Promise<TaskEventDetail | null>;
  cancelled(): Promise<TaskEventDetail | null>;
  /** As the Cancel or Stop button. */
  stop(): Promise<void>;
  /** A part of the task, with its own target; the task ends, with one summary, when every part has. */
  part(spec: Omit<TaskSpec, 'control'>): TaskHandle;
}
export interface TaskBindings {
  Tasks?(): Promise<TaskSnapshotLike[]>; Get?(id: string): Promise<TaskSnapshotLike>;
  Stop?(id: string): Promise<void>; Pause?(id: string): Promise<void>; Resume?(id: string): Promise<void>;
  Retry?(id: string): Promise<void>; Dismiss?(id: string): Promise<void>; DismissFinished?(): Promise<number>;
}
export interface TaskFollower { readonly id: string; readonly state: TaskState | null; stop(): Promise<void> | undefined; unfollow(): void }
export const wkTask: {
  (spec: TaskSpec): TaskHandle;
  connect(options: { subscribe: (listener: (payload: { tasks: TaskSnapshotLike[] }) => void) => (() => void) | void; bindings?: TaskBindings; confirmStop?: (snapshot: TaskSnapshotLike) => boolean | Promise<boolean> }): () => void;
  follow(id: string, options?: { control?: Element | Element[]; target?: Element; bar?: HTMLElement; scope?: Element; stopIn?: Element; expect?: 'short' | 'maybe-long' | 'long' }): TaskFollower;
  /** The summary button; dispose() stops it updating. */
  summary(): HTMLButtonElement & { dispose(): void };
  showList(options?: { opener?: Element }): WkDialog;
  words: Record<string, string | ((...args: any[]) => string)>;
};

// ---- Chart (chart.js) ----
export interface ChartCategory { label: string; name?: string; shaded?: boolean }
export interface ChartSeries { key?: string; label: string; tone?: number | string; values: (number | null)[] }
export interface ChartSpec {
  id?: string; kind: 'bars' | 'line'; label: string; summary?: string; caption?: string;
  categories: ChartCategory[]; series: ChartSeries[]; band?: ChartSeries;
  stacked?: boolean; orientation?: 'vertical' | 'horizontal';
  format?: (v: number) => string; tableFormat?: (v: number) => string; ticks?: (max: number) => number[]; integer?: boolean;
  legendValues?: boolean; shadeLabel?: string; categoryHeader?: string; emptyText?: string;
  view?: 'chart' | 'table'; onview?: (view: 'chart' | 'table') => void;
  selection?: number; onselection?: (index: number) => void; retainSelectionOnBlur?: boolean;
  /** Loading: the frame and legend stay, the plot shows a spinner named by these words after 400 ms. */
  loading?: string;
}
export const wkChart: {
  (spec: ChartSpec): HTMLElement;
  /** Resolves when a loading chart may be replaced: at once if its spinner hasn't shown, else after 500 ms of it. */
  ready(fig: HTMLElement): Promise<void>;
};
export function niceTicks(max: number, count?: number, integer?: boolean): number[];
export function labelStep(band: number, width: number): number;
export function runs(values: (number | null)[]): [number, number][][];
export function roundedTop(x: number, y: number, w: number, h: number, r?: number): string;
export function roundedEnd(x: number, y: number, w: number, h: number, r?: number): string;
export function tableModel(spec: ChartSpec): { head: string[]; rows: string[][] };
export function readout(spec: ChartSpec, index: number): { title: string; rows: { tone?: number | string; shape: string; label: string; text: string }[]; text: string };

// ---- Diff (diff.js) ----
export type DiffLineKind = 'context' | 'add' | 'del';
export interface DiffLine { kind: DiffLineKind; old: number | null; new: number | null; text: string; noNewline?: boolean }
export interface DiffHunk { header: string; oldStart: number; oldLines: number; newStart: number; newLines: number; section: string; lines: DiffLine[] }
export type DiffStatus = 'modified' | 'added' | 'deleted' | 'renamed' | 'copied';
export interface DiffFile {
  path: string; oldPath: string; status: DiffStatus; binary: boolean;
  oldMode: string | null; newMode: string | null; additions: number; deletions: number; hunks: DiffHunk[];
  /** Set by the app on parsed input: start collapsed as generated, or force collapsed or expanded. */
  generated?: boolean; collapsed?: boolean;
}
/** A file the app parsed itself: only the path is required; the rest is filled in. A line of any other kind is shown as context; lines without numbers are numbered from the hunk's start. */
export interface DiffFileInput {
  path: string; oldPath?: string; status?: DiffStatus | 'binary'; binary?: boolean; generated?: boolean; collapsed?: boolean;
  oldMode?: string | null; newMode?: string | null; additions?: number; deletions?: number;
  hunks?: { header?: string; oldStart?: number; oldLines?: number; newStart?: number; newLines?: number; section?: string; lines: (Omit<DiffLine, 'old' | 'new' | 'kind'> & { kind: DiffLineKind | string; old?: number | null; new?: number | null })[] }[];
}
export interface DiffContextRange { oldStart: number; oldEnd: number; newStart: number; newEnd: number }
export interface DiffSpec {
  id?: string; label?: string; diff: string | DiffFileInput[]; emptyText?: string;
  view?: 'unified' | 'split'; onview?: (view: 'unified' | 'split') => void;
  wrap?: boolean; collapseAbove?: number; collapseCharsAbove?: number; generated?: (file: DiffFile) => boolean; headingLevel?: 2 | 3 | 4 | 5 | 6;
  highlight?: (file: DiffFile, line: DiffLine) => Node | string | null | undefined;
  getContext?: (file: DiffFile, range: DiffContextRange) => string[] | null | Promise<string[] | null>;
  contextStep?: number;
  /** Called after a file's rows are built: when it is first expanded, and on every rebuild (a view switch) or added context. */
  onrender?: (fileElement: HTMLElement, file: DiffFile) => void;
  /** Review threads, drawn under their lines (see README, "Review comments"). */
  comments?: DiffThread[];
  /** Whether a line takes a new comment; needs oncomment. Default true. */
  canComment?: boolean | ((file: DiffFile, side: DiffSide, line: number) => boolean);
  /** The comment box's factory; the default is a wk-field textarea. */
  commentBox?: (spec: DiffCommentBoxSpec) => DiffCommentBox | HTMLElement;
  commentPlaceholder?: string;
  /** A comment's body as a node; the default is its plain text. */
  renderBody?: (entry: DiffCommentEntry) => Node | null | undefined;
  /** A promise shows the comment as pending until it settles; a rejection shows the error with Retry. */
  oncomment?: (comment: DiffNewComment) => unknown;
  onreply?: (threadId: string, body: string) => unknown;
  /** A promise makes Resolve busy until it settles. */
  onresolve?: (threadId: string, resolved: boolean) => unknown;
  /** Retry on an entry the app marked with error. */
  onretry?: (threadId: string, entry: DiffCommentEntry) => void;
  /** Who a comment being posted shows as. Default "You". */
  author?: { name?: string; avatar?: string };
  /** The clock relative times are measured from, in ms. Default Date.now. */
  now?: () => number;
}
export type DiffSide = 'old' | 'new';
export interface DiffCommentEntry { author: string; avatar?: string; time: Date | string | number; body: string; bodyHtml?: string; pending?: boolean; error?: string }
export interface DiffThread { id: string; path: string; side: DiffSide; line: number; endLine?: number; entries: DiffCommentEntry[]; resolved?: boolean; outdated?: boolean }
export interface DiffNewComment { path: string; side: DiffSide; line: number; endLine: number; body: string }
export interface DiffCommentBoxSpec { value: string; placeholder: string; label: string; onSubmit: () => void; onCancel: () => void }
/** value may also be settable: Edit then adds a failed comment's text to a box already open. */
export interface DiffCommentBox { element: HTMLElement; value: string; focus?: () => void; destroy?: () => void }
export interface DiffCommentPlace { thread: DiffThread; side: DiffSide; line: number; endLine: number; hunk?: number; outdated?: true }
export function wkDiff(spec: DiffSpec): HTMLElement & { setComments(threads: DiffThread[]): void; destroy(): void };
export function diffCommentPlaces(file: DiffFile, threads: DiffThread[]): { placed: DiffCommentPlace[]; outdated: DiffCommentPlace[] };
export function parseUnifiedDiff(text: string): DiffFile[];
export function diffSplitRows(lines: DiffLine[]): { left: DiffLine | null; right: DiffLine | null }[];
export function diffWordRanges(before: string, after: string, limit?: number): { del: [number, number][]; add: [number, number][] };

// ---- Settings window (settings-model.js, settings.js) ----
export const SECRET_MASK: string;
export function conditionMet(condition: Condition | undefined, values: Values): boolean;
export function resolveOptions(field: Field, values: Values): SelectOption[];
export function fieldValue(field: Field, values: Values): unknown;
export function readValue(field: Field, raw: unknown): unknown;
export function displayValue(field: Field, value: unknown): string;
export function dependentResets(schema: Schema, key: string, values: Values): Values;
export function errorsByField(errors: ValidationError[] | null | undefined): Map<string, string>;
export interface SettingsRow {
  key: string; type: FieldType; label: string; description: string; placeholder: string; visible: boolean;
  value: unknown; text: string; options: SelectOption[] | null; missing: boolean; disabled: boolean;
  error: string | null; min: number | null; max: number | null; required: boolean;
}
export function settingsModel(schema: Schema, values?: Values, errors?: ValidationError[] | null): {
  groups: { key: string; label: string; attention: boolean; rows: SettingsRow[]; advanced: SettingsRow[] }[];
  /** The number of visible fields with an error. */
  attention: number;
};
export interface SettingsChangeDetail { key: string; value: unknown; values: Values; errors: ValidationError[]; checked: boolean }
export class WkSettings extends HTMLElement {
  schema: Schema | null;
  values: Values;
  errors: ValidationError[];
  icons: Record<string, string | Node>;
  validate: ((schema: Schema, values: Values) => ValidationError[]) | null;
  /** A back-end check, such as settings.Service.CheckValue: the field's errors, or none when it passes (and has saved). */
  check: ((key: string, values: Values) => Promise<ValidationError[] | null>) | null;
  /** Selects the field's tab, opens Advanced if needed and focuses its control; false when the field isn't shown. */
  reveal(key: string): boolean;
}

// ---- Toast, undo, copy and skeleton rows (toast.js, undo.js, copy.js, skeleton.js) ----
export interface ToastAction {
  label: string;
  run(): unknown;
  /** Set by wkUndo.action(): the toast closes once the undo stack moves past this action. */
  undoId?: string | null;
  current?(): string | null;
}
export function wkToast(options: { words: string; action?: ToastAction }): { element: HTMLElement; dismiss(): void };

export interface UndoEntryLike { id: string; name: string; label: string }
export interface UndoStateLike { undo: UndoEntryLike | null; redo: UndoEntryLike | null; busy: boolean; textFocus: boolean }
/** The Go UndoStack's Bindings(), as Wails binds it. */
export interface UndoBindings {
  State?(): Promise<UndoStateLike>; Undo?(): Promise<void>; Redo?(): Promise<void>;
  UndoID?(id: string): Promise<void>; SetTextFocus?(focused: boolean): Promise<void>;
}
export const wkUndo: {
  /** on(name, listener) subscribes to a Go event (undo:changed, undo:text, undo:failed) and may return an unsubscribe. */
  connect(options: { on: (name: string, listener: (payload: any) => void) => (() => void) | void; bindings?: UndoBindings }): () => void;
  state(): UndoStateLike;
  undo(): Promise<boolean>;
  redo(): Promise<boolean>;
  /** A toast's Undo for the action with this ID (else the most recent). */
  action(id?: string): Required<ToastAction>;
  words: { undo: string; busy: string; couldNot: (op: 'undo' | 'redo', name: string) => string };
};

export const wkCopy: {
  (button: HTMLElement, text: string): Promise<boolean>;
  /** The clipboard writer; replace it with Wails's Clipboard.SetText, say. Rejecting, or resolving to false, is a failure. */
  write: (text: string) => Promise<unknown>;
  words: { copied: string; failed: string };
};

export function wkSkeleton(body: HTMLTableSectionElement | HTMLTableElement, options?: { rows?: number; columns?: number; region?: Element }): {
  /** Takes the skeleton rows out, calls draw in the same step, and resolves; waits for the 500 ms minimum once they've shown. */
  done(draw?: () => void): Promise<void>;
  /** Takes them out and puts the old rows back. */
  cancel(): Promise<void>;
  readonly shown: boolean;
};

declare global {
  interface HTMLElementTagNameMap {
    'wk-menu': WkMenu; 'wk-context-menu': WkContextMenu; 'wk-tabs': WkTabs; 'wk-tooltip': WkTooltip;
    'wk-dialog': WkDialog; 'wk-settings': WkSettings;
  }
  interface HTMLElementEventMap { 'wk-settings-change': CustomEvent<SettingsChangeDetail>; 'wk-task': CustomEvent<TaskEventDetail>; 'wk-copy': CustomEvent<{ ok: boolean }> }
  interface DocumentEventMap { 'wk-undo-change': CustomEvent<UndoStateLike> }
}
