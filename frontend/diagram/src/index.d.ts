// Types for @wails-kit/diagram (dist/diagram.js).

/** The diagram types the kit draws. */
export type DiagramType = 'flowchart' | 'sequence' | 'state' | 'class' | 'er' | 'gantt';

/** Why a diagram was not drawn. */
export interface DiagramFailure {
  /** Mermaid's message (with its line and a pointer for a syntax error), or the kit's. */
  message: string;
  /** The source line of a syntax error, when Mermaid gives one. */
  line: number | null;
  /** 'syntax': Mermaid could not parse or draw it; 'unsupported': a type the kit does not draw; 'load': dist/mermaid.js did not load. */
  kind: 'syntax' | 'unsupported' | 'load';
}

export interface DiagramExport {
  format: 'svg' | 'png';
  blob: Blob;
  /** `<filename or label>.svg` or `.png`. */
  filename: string;
}

export interface DiagramSpec {
  /** The accessible name. Required. */
  label: string;
  /** Mermaid text. */
  source?: string;
  /** A text alternative. Default: the source itself, in a details element. */
  description?: string;
  id?: string;
  /** The exported file's name without its extension. Default: the label, as a slug. */
  filename?: string;
  /** Called when the diagram cannot be drawn (the ErrorBox shows either way). */
  onerror?: (failure: DiagramFailure) => void;
  /** Called by Save SVG and Save PNG instead of downloading the file. */
  onexport?: (file: DiagramExport) => void;
}

export type DiagramRenderEvent = CustomEvent<{ type: DiagramType }>;
export type DiagramErrorEvent = CustomEvent<DiagramFailure>;

export interface DiagramElement extends HTMLDivElement {
  /** The Mermaid text. Setting it re-renders. */
  source: string;
  /** Settles when the diagram shows the current source in the current appearance and theme (drawn, or the ErrorBox shown); it re-renders first if the theme changed and the change has not been picked up yet. */
  readonly ready: Promise<void>;
  /** The zoom level (1 is 100%). */
  readonly zoom: number;
  /** The type of the diagram shown, or null. */
  readonly type: DiagramType | null;
  /** Fits the diagram in its box, centred, never above 100%. */
  fit(): void;
  /** Zooms to `scale` (0.1 to 8) around the middle. */
  zoomTo(scale: number): void;
  /** Zooms by `factor` around the middle. */
  zoomBy(factor: number): void;
  /** A standalone SVG of the diagram, with its styles, a title and a description. Throws with no diagram. */
  toSVG(): string;
  /** A PNG of the diagram at `scale` (default 2). Needs img-src data: in the page's CSP. */
  toPNG(scale?: number): Promise<Blob>;
  /** Copies the source to the clipboard; resolves false if it could not. */
  copySource(): Promise<boolean>;
  /** Tears the diagram down; the element is left empty. */
  destroy(): void;
  addEventListener(type: 'wk-render', listener: (this: DiagramElement, ev: DiagramRenderEvent) => void, options?: boolean | AddEventListenerOptions): void;
  addEventListener(type: 'wk-error', listener: (this: DiagramElement, ev: DiagramErrorEvent) => void, options?: boolean | AddEventListenerOptions): void;
  addEventListener<K extends keyof HTMLElementEventMap>(type: K, listener: (this: HTMLDivElement, ev: HTMLElementEventMap[K]) => void, options?: boolean | AddEventListenerOptions): void;
  addEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | AddEventListenerOptions): void;
  removeEventListener(type: 'wk-render', listener: (this: DiagramElement, ev: DiagramRenderEvent) => void, options?: boolean | EventListenerOptions): void;
  removeEventListener(type: 'wk-error', listener: (this: DiagramElement, ev: DiagramErrorEvent) => void, options?: boolean | EventListenerOptions): void;
  removeEventListener<K extends keyof HTMLElementEventMap>(type: K, listener: (this: HTMLDivElement, ev: HTMLElementEventMap[K]) => void, options?: boolean | EventListenerOptions): void;
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | EventListenerOptions): void;
}

/** Builds a diagram. Mermaid (dist/mermaid.js) loads on the first call's render. */
export function wkDiagram(spec: DiagramSpec): DiagramElement;

/** Loads Mermaid ahead of the first diagram. */
export function preloadDiagrams(): Promise<void>;

/** The diagram types the kit draws. */
export const diagramTypes: readonly DiagramType[];

/** The indicator timings from @wails-kit/ui's BEHAVIOUR.md, in milliseconds. */
export const TIMINGS: Readonly<{ indicatorDelay: number; indicatorMinimum: number; loadingLine: number; copied: number }>;

export interface DiagramTheme {
  themeVariables: Record<string, string | boolean>;
  fontFamily: string;
  fontSize: number;
  darkMode: boolean;
}
/** Mermaid's theme from the tokens; `get(name)` returns a token's value. */
export function buildTheme(get: (name: string) => string, options?: { forced?: { canvas: string; text: string } | null }): DiagramTheme;
/** The tokens buildTheme reads. */
export const TOKENS: readonly string[];
/** [text, ground] theme variable pairs that reach 4.5:1. */
export const TEXT_PAIRS: readonly (readonly [string, string])[];
/** [mark, ground] theme variable pairs that reach 3:1. */
export const MARK_PAIRS: readonly (readonly [string, string])[];
/** WCAG contrast of two opaque colours. */
export function contrast(a: string | number[], b: string | number[]): number;
