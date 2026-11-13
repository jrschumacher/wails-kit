// The settings window's rules, with no DOM, so node can test them
// (test/settings.test.mjs). They read wails-kit's settings schema: the JSON
// that settings.Service.GetSchema() returns (frontend/types settings.ts).
// <wk-settings> (settings.js) draws what settingsModel() describes.

/** What GetValues() returns for a password field that holds a secret. Sending it back leaves the secret unchanged. */
export const SECRET_MASK = '••••••••';

/** Whether a field with this condition shows: no condition, or the controlling field's value is one of condition.equals. */
export function conditionMet(condition, values) {
  if (!condition) return true;
  const value = String(values?.[condition.field] ?? '');
  return (condition.equals || []).some(eq => value === eq);
}

/** The options a select field offers now: its dynamic options for the controlling field's value, else its static options. */
export function resolveOptions(field, values) {
  if (field.dynamicOptions) {
    const on = String(values?.[field.dynamicOptions.dependsOn] ?? '');
    return field.dynamicOptions.options?.[on] ?? [];
  }
  return field.options ?? [];
}

/** The first message for each field key, from settings.ValidationError[] (or null). */
export function errorsByField(errors) {
  const out = new Map();
  for (const e of errors || []) if (e && e.field && !out.has(e.field)) out.set(e.field, e.message || e.code || 'Invalid value');
  return out;
}

const fallback = field => {
  if (field.default !== undefined && field.default !== null) return field.default;
  if (field.type === 'toggle') return false;
  if (field.type === 'number') return null;
  return '';
};

/** A field's value: values[key], else its default, else false for a toggle, null for a number and "" otherwise. */
export function fieldValue(field, values) {
  const v = values?.[field.key];
  return v === undefined ? fallback(field) : v;
}

/**
 * The value a control holds, as the schema's type: a toggle's checked state
 * as a boolean, a number field's text as a number (null when empty or not a
 * number), anything else as a string.
 */
export function readValue(field, raw) {
  if (field.type === 'toggle') return Boolean(raw);
  if (field.type === 'number') {
    const text = String(raw ?? '').trim();
    if (text === '') return null;
    const n = Number(text);
    return Number.isFinite(n) ? n : null;
  }
  return raw == null ? '' : String(raw);
}

/** A value as text for a control or a read-only row. */
export function displayValue(field, value) {
  if (value == null) return '';
  if (field.type === 'toggle') return value ? 'On' : 'Off';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

/**
 * The selects whose options follow `key`, with the value each must take now:
 * when a controlling field changes, a dependent select whose value is no
 * longer offered moves to its first option ("" when it offers none).
 */
export function dependentResets(schema, key, values) {
  const out = {};
  for (const group of schema?.groups || []) {
    for (const field of group.fields || []) {
      if (field.type !== 'select' || field.dynamicOptions?.dependsOn !== key) continue;
      const options = resolveOptions(field, values);
      const value = String(fieldValue(field, values) ?? '');
      if (!options.some(o => o.value === value)) out[field.key] = options[0]?.value ?? '';
    }
  }
  return out;
}

/**
 * What the window shows for a schema, its values and the errors to show:
 *   { groups: [{ key, label, attention, rows, advanced }] }
 * rows and advanced hold one row per field, in schema order (advanced holds
 * the fields marked advanced): { key, type, label, description, placeholder,
 * visible, value, text, options, missing, disabled, error, min, max,
 * required }. attention is true when a visible field in the group has an
 * error. A select whose value is not among its options has missing: true; a
 * select with no options is disabled.
 */
export function settingsModel(schema, values = {}, errors = []) {
  const byField = errorsByField(errors);
  const groups = (schema?.groups || []).map(group => {
    const rows = [], advanced = [];
    for (const field of group.fields || []) {
      const visible = conditionMet(field.condition, values);
      const value = fieldValue(field, values);
      const v = field.validation || {};
      const row = {
        key: field.key,
        type: field.type,
        label: field.label || field.key,
        description: field.description || '',
        placeholder: field.placeholder || '',
        visible,
        value,
        text: displayValue(field, value),
        options: null,
        missing: false,
        disabled: false,
        error: visible ? byField.get(field.key) ?? null : null,
        min: v.min ?? null,
        max: v.max ?? null,
        required: Boolean(v.required),
      };
      if (field.type === 'select') {
        row.options = resolveOptions(field, values);
        row.missing = !row.options.some(o => o.value === String(value ?? ''));
        row.disabled = row.options.length === 0;
      }
      (field.advanced ? advanced : rows).push(row);
    }
    const attention = [...rows, ...advanced].some(r => r.error);
    return { key: group.key, label: group.label || group.key, attention, rows, advanced };
  });
  return { groups };
}
