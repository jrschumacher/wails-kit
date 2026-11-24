// Types for @wails-kit/ui (optional: the package is plain JavaScript).
// Settings shapes mirror frontend/types settings.ts and the Go settings package.

export interface SelectOption { label: string; value: string }
export interface Condition { field: string; equals: string[] }
export interface Validation { required?: boolean; pattern?: string; minLen?: number; maxLen?: number; min?: number; max?: number }
export type FieldType = 'text' | 'password' | 'select' | 'toggle' | 'computed' | 'number';
export interface Field {
  key: string; type: FieldType; label: string; description?: string; placeholder?: string; default?: unknown;
  options?: SelectOption[]; dynamicOptions?: { dependsOn: string; options: Record<string, SelectOption[]> };
  condition?: Condition; validation?: Validation; advanced?: boolean;
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
}
export function wkChart(spec: ChartSpec): HTMLElement;
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
  onresolve?: (threadId: string, resolved: boolean) => void;
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
export interface DiffCommentBox { element: HTMLElement; readonly value: string; focus?: () => void; destroy?: () => void }
export interface DiffCommentPlace { thread: DiffThread; side: DiffSide; line: number; endLine: number; hunk?: number; outdated?: true }
export function wkDiff(spec: DiffSpec): HTMLElement & { setComments(threads: DiffThread[]): void };
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
};
export interface SettingsChangeDetail { key: string; value: unknown; values: Values; errors: ValidationError[] }
export class WkSettings extends HTMLElement {
  schema: Schema | null;
  values: Values;
  errors: ValidationError[];
  icons: Record<string, string | Node>;
  validate: ((schema: Schema, values: Values) => ValidationError[]) | null;
}

declare global {
  interface HTMLElementTagNameMap {
    'wk-menu': WkMenu; 'wk-context-menu': WkContextMenu; 'wk-tabs': WkTabs; 'wk-tooltip': WkTooltip;
    'wk-dialog': WkDialog; 'wk-settings': WkSettings;
  }
  interface HTMLElementEventMap { 'wk-settings-change': CustomEvent<SettingsChangeDetail> }
}
