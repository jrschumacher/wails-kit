// Types for @wails-kit/editor/richtext (dist/richtext.js).

/** What the editor offers. Content it does not offer is cut down to plain text when loaded or pasted. */
export type RichTextFeature =
  | 'bold' | 'italic' | 'code' | 'link'
  | 'heading' | 'bulletList' | 'orderedList' | 'quote' | 'codeBlock' | 'rule';

/** Every feature, in toolbar order. */
export const FEATURES: readonly RichTextFeature[];

export type RichTextFormat = 'markdown' | 'html';

export interface RichTextEditorAction {
  label: string;
  /** The kit's primary button. */
  primary?: boolean;
  /** Submits (as Mod-Enter does): checks the text, fires wk-submit, calls onsubmit. */
  submit?: boolean;
  /** Cancels (as Escape does): fires wk-cancel, calls oncancel. Stays usable while the editor is disabled. */
  cancel?: boolean;
  /** Called with the editor when pressed (not for a submit or cancel action). */
  onclick?: (editor: RichTextEditorElement) => void;
}

export interface RichTextEditorSpec {
  /** The accessible name of the text. Required. */
  label: string;
  id?: string;
  /** The content, in `format`. */
  value?: string;
  /** What `value` reads and writes: Markdown (the default) or sanitised HTML. */
  format?: RichTextFormat;
  placeholder?: string;
  /** The features to offer, in any order. Default all of them. An unknown name throws. */
  features?: readonly RichTextFeature[];
  /** The formatting toolbar. Default true; the shortcuts work without it. */
  toolbar?: boolean;
  /** The text can be read, selected and copied, and its links followed, but not changed. No toolbar. */
  readOnly?: boolean;
  disabled?: boolean;
  /** A CSS length, or a number of pixels: the text grows with its content from this… Default 100px. */
  minHeight?: string | number;
  /** …to this, then scrolls. Default 360px. */
  maxHeight?: string | number;
  /** An empty text does not submit (it shows an error). Default true. */
  required?: boolean;
  /** The buttons under the editor, in order (e.g. Cancel, then a primary submit). */
  actions?: readonly RichTextEditorAction[];
  /** Called with the value after each edit (not when the app sets `value`). */
  onchange?: (value: string) => void;
  /** Called with the value on submit. Return a promise to keep the submit buttons busy until it settles. */
  onsubmit?: (value: string) => unknown;
  /** Called with the editor on cancel: a `cancel` action, cancel(), or Escape (only when this is set). */
  oncancel?: (editor: RichTextEditorElement) => void;
  /** Called with a link's URL (http, https or mailto) when the person opens it; the page never navigates. */
  onlink?: (url: string) => void;
}

export type RichTextChangeEvent = CustomEvent<{ value: string }>;
export type RichTextSubmitEvent = CustomEvent<{ value: string }>;
export type RichTextCancelEvent = CustomEvent<null>;

interface RichTextEditorEventMap {
  'wk-change': RichTextChangeEvent;
  'wk-submit': RichTextSubmitEvent;
  'wk-cancel': RichTextCancelEvent;
}

/** The element wkRichTextEditor returns. */
export interface RichTextEditorElement extends HTMLDivElement {
  /** The content in `format`. Setting it replaces the content and its undo history, clears an error and fires nothing. */
  value: string;
  readonly format: RichTextFormat;
  /** The text cannot be changed, and the toolbar and the actions are disabled. */
  disabled: boolean;
  focus(): void;
  /** Submits as the submit action does; false when it did not (an error, disabled, busy or cancelled). */
  submit(): boolean;
  /** Cancels: fires wk-cancel, then calls oncancel. */
  cancel(): void;
  /** Tears the editor down; the element is left empty. */
  destroy(): void;
  addEventListener<K extends keyof RichTextEditorEventMap>(type: K, listener: (this: RichTextEditorElement, ev: RichTextEditorEventMap[K]) => void, options?: boolean | AddEventListenerOptions): void;
  addEventListener<K extends keyof HTMLElementEventMap>(type: K, listener: (this: HTMLDivElement, ev: HTMLElementEventMap[K]) => void, options?: boolean | AddEventListenerOptions): void;
  addEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | AddEventListenerOptions): void;
  removeEventListener<K extends keyof RichTextEditorEventMap>(type: K, listener: (this: RichTextEditorElement, ev: RichTextEditorEventMap[K]) => void, options?: boolean | EventListenerOptions): void;
  removeEventListener<K extends keyof HTMLElementEventMap>(type: K, listener: (this: HTMLDivElement, ev: HTMLElementEventMap[K]) => void, options?: boolean | EventListenerOptions): void;
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | EventListenerOptions): void;
}

export function wkRichTextEditor(spec: RichTextEditorSpec): RichTextEditorElement;
