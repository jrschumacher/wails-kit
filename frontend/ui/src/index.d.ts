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
  selection?: number; onselection?: (index: number) => void;
}
export function wkChart(spec: ChartSpec): HTMLElement;
export function niceTicks(max: number, count?: number, integer?: boolean): number[];
export function labelStep(band: number, width: number): number;
export function runs(values: (number | null)[]): [number, number][][];
export function roundedTop(x: number, y: number, w: number, h: number, r?: number): string;
export function roundedEnd(x: number, y: number, w: number, h: number, r?: number): string;
export function tableModel(spec: ChartSpec): { head: string[]; rows: string[][] };
export function readout(spec: ChartSpec, index: number): { title: string; rows: { tone?: number | string; shape: string; label: string; text: string }[]; text: string };

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
