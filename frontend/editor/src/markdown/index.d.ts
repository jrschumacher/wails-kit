// Types for @wails-kit/editor/markdown (dist/markdown.js).

export type MarkdownMode = 'write' | 'preview';

export interface RenderMarkdownOptions {
  /** Called with a link's URL when it is clicked; the page never navigates. Open it in the system browser. */
  onlink?: (url: string) => void;
  /** Hosts whose https images load (exact names, e.g. "images.example.com"). Default none: an image renders as a link to it. */
  imageHosts?: readonly string[];
  /** Colour fenced code in a bundled language. Default true. */
  highlight?: boolean;
}

/**
 * Markdown (CommonMark with GFM tables, task lists, strikethrough and
 * autolinks) as a DocumentFragment, safe for untrusted text: no raw HTML,
 * links only to http, https and mailto URLs, no remote images unless allowed.
 * Put it in an element with the class `wk-markdown`.
 */
export function renderMarkdown(text: string, options?: RenderMarkdownOptions): DocumentFragment;

export interface MarkdownEditorAction {
  label: string;
  /** The kit's primary button. */
  primary?: boolean;
  /** Submits (as Mod-Enter does): checks the text, fires wk-submit, calls onsubmit. */
  submit?: boolean;
  /** Called with the editor when pressed (not for a submit action). */
  onclick?: (editor: MarkdownEditorElement) => void;
}

export interface MarkdownEditorSpec {
  /** The accessible name of the text and of the tab list. Required. */
  label: string;
  id?: string;
  value?: string;
  placeholder?: string;
  /** Start in this mode. Default "write". */
  mode?: MarkdownMode;
  /** The formatting toolbar. Default true; the shortcuts work without it. */
  toolbar?: boolean;
  /** A CSS length: the text grows with its content from this… Default 100px. */
  minHeight?: string;
  /** …to this, then scrolls. Default 360px. */
  maxHeight?: string;
  /** Shows a live count against this many characters; submitting a longer text shows an error instead. */
  maxLength?: number;
  /** An empty text does not submit (it shows an error). Default true. */
  required?: boolean;
  disabled?: boolean;
  /** The buttons under the editor, in order (e.g. Cancel, then a primary submit). */
  actions?: readonly MarkdownEditorAction[];
  /** Called with the text after each edit (not when the app sets `value`). */
  onchange?: (value: string) => void;
  /** Called with the text on submit. Return a promise to keep the submit buttons busy until it settles. */
  onsubmit?: (value: string) => unknown;
  /** Called when the person switches between Write and Preview. */
  onmode?: (mode: MarkdownMode) => void;
  /** As renderMarkdown's: a preview link's click. */
  onlink?: (url: string) => void;
  /** As renderMarkdown's. */
  imageHosts?: readonly string[];
}

export type MarkdownChangeEvent = CustomEvent<{ value: string }>;
export type MarkdownModeEvent = CustomEvent<{ mode: MarkdownMode }>;
export type MarkdownSubmitEvent = CustomEvent<{ value: string }>;

interface MarkdownEditorEventMap {
  'wk-change': MarkdownChangeEvent;
  'wk-mode': MarkdownModeEvent;
  'wk-submit': MarkdownSubmitEvent;
}

/** The element wkMarkdownEditor returns. */
export interface MarkdownEditorElement extends HTMLDivElement {
  /** The text. Setting it replaces the text and its undo history, clears an error and fires nothing. */
  value: string;
  /** The shown tab. Setting it fires nothing. */
  mode: MarkdownMode;
  /** Read-only text, with the toolbar and the actions disabled. */
  disabled: boolean;
  /** Shows Write and focuses the text. */
  focus(): void;
  /** Submits as the submit action does; false when it did not (an error, disabled, busy or cancelled). */
  submit(): boolean;
  /** Tears the editor down; the element is left empty. */
  destroy(): void;
  addEventListener<K extends keyof MarkdownEditorEventMap>(type: K, listener: (this: MarkdownEditorElement, ev: MarkdownEditorEventMap[K]) => void, options?: boolean | AddEventListenerOptions): void;
  addEventListener<K extends keyof HTMLElementEventMap>(type: K, listener: (this: HTMLDivElement, ev: HTMLElementEventMap[K]) => void, options?: boolean | AddEventListenerOptions): void;
  addEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | AddEventListenerOptions): void;
  removeEventListener<K extends keyof MarkdownEditorEventMap>(type: K, listener: (this: MarkdownEditorElement, ev: MarkdownEditorEventMap[K]) => void, options?: boolean | EventListenerOptions): void;
  removeEventListener<K extends keyof HTMLElementEventMap>(type: K, listener: (this: HTMLDivElement, ev: HTMLElementEventMap[K]) => void, options?: boolean | EventListenerOptions): void;
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | EventListenerOptions): void;
}

export function wkMarkdownEditor(spec: MarkdownEditorSpec): MarkdownEditorElement;
