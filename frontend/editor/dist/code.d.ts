// Types for @wails-kit/editor/code (dist/code.js).

/** A CodeMirror stream parser (a legacy mode, e.g. from @codemirror/legacy-modes). */
export interface StreamParser {
  name?: string;
  token(stream: unknown, state: unknown): string | null;
  [key: string]: unknown;
}

export type BundledLanguage =
  | 'javascript' | 'typescript' | 'go' | 'json' | 'yaml' | 'markdown' | 'shell' | 'css' | 'html';

export interface CodeEditorSpec {
  /** The accessible name. Required. */
  label: string;
  id?: string;
  value?: string;
  /** A bundled language name or alias (js, ts, yml, md, sh, …), a stream parser, or none for plain text. */
  language?: BundledLanguage | (string & {}) | StreamParser | null;
  readOnly?: boolean;
  /** Line numbers in a gutter. Default true. */
  lineNumbers?: boolean;
  /** Wrap long lines. Default false. */
  wrap?: boolean;
  /** Columns per tab and per indent. Default 4 (Go indents with tabs). */
  tabSize?: number;
  placeholder?: string;
  /** Tab indents instead of moving focus; Escape then Tab moves focus. Default false. */
  tabIndents?: boolean;
  /** Called with the new text after each edit (not when the app sets `value`). */
  onchange?: (value: string) => void;
}

/** The bubbling event after each edit. */
export type CodeEditorChangeEvent = CustomEvent<{ value: string }>;

/**
 * The element wkCodeEditor returns. Its `wk-change` listener is typed here,
 * not in the global HTMLElementEventMap: other kit elements (wk-tabs) fire
 * `wk-change` with a different detail.
 */
export interface CodeEditorElement extends HTMLDivElement {
  /** The text. Setting it replaces the document and its undo history; after destroy() it does nothing. */
  value: string;
  /** Focuses the text; after destroy() it does nothing. */
  focus(): void;
  /** Tears the editor down; the element is left empty. */
  destroy(): void;
  addEventListener(type: 'wk-change', listener: (this: CodeEditorElement, ev: CodeEditorChangeEvent) => void, options?: boolean | AddEventListenerOptions): void;
  addEventListener<K extends keyof HTMLElementEventMap>(type: K, listener: (this: HTMLDivElement, ev: HTMLElementEventMap[K]) => void, options?: boolean | AddEventListenerOptions): void;
  addEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | AddEventListenerOptions): void;
  removeEventListener(type: 'wk-change', listener: (this: CodeEditorElement, ev: CodeEditorChangeEvent) => void, options?: boolean | EventListenerOptions): void;
  removeEventListener<K extends keyof HTMLElementEventMap>(type: K, listener: (this: HTMLDivElement, ev: HTMLElementEventMap[K]) => void, options?: boolean | EventListenerOptions): void;
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | EventListenerOptions): void;
}

export function wkCodeEditor(spec: CodeEditorSpec): CodeEditorElement;

/** A diff file as @wails-kit/ui's diff view passes it (only the paths are read). */
export interface HighlightFile { path?: string; oldPath?: string }
/** A diff line (only `text` is read), or the text itself. */
export type HighlightLine = { text: string } | string;
export type Highlight = (file: HighlightFile | null | undefined, line: HighlightLine) => DocumentFragment;

/**
 * The diff view's highlight hook, in the editor's syntax colours. With no
 * language it picks one per file from the path's extension.
 */
export function highlighter(language?: BundledLanguage | (string & {})): Highlight;

/** The bundled language names. */
export const languages: readonly BundledLanguage[];
/** The bundled language for a file path, by its extension (`src/app.tsx` → "typescript"), or null. */
export function languageForPath(path: string): BundledLanguage | null;
/** The bundled language a name or alias stands for (`yml` → "yaml"), or null. */
export function languageName(name: string): BundledLanguage | null;
