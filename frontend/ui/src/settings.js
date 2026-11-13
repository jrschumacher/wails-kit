// <wk-settings>: a settings window drawn from wails-kit's settings schema
// (settings.Service.GetSchema()). Light DOM, built from the kit's own pieces:
// settings tabs (one per group, through <wk-tabs>) when there is more than one
// group, a wk-panel--rows of wk-settings-row per group, an "Advanced"
// disclosure for fields marked advanced, and kit controls per field type:
//
//   text      <input class="wk-field">
//   password  <input class="wk-field" type="password">; shows the secret mask
//             when a secret is stored, and sends it back unchanged unless edited
//   select    <select class="wk-select">; dynamic options follow their field
//   toggle    <input class="wk-switch" role="switch">, the whole row its label
//   number    a wk-stepper, with validation.min and max as its limits
//   computed  read-only text
//
// Data in through properties: schema, values, errors (settings.ValidationError[]
// from SetValues), icons ({ groupKey: svg markup or a Node }) and validate (an
// optional (schema, values) => errors, such as @wails-kit/settings' validate).
// Data out through one bubbling event, wk-settings-change, fired when a control
// commits a value: detail { key, value, values, errors }. values holds every
// field; errors is what validate returned ([] without it). The app saves
// (SetValues) and sets errors from the reply.
import { SECRET_MASK, settingsModel, readValue, dependentResets } from './settings-model.js';
import { syncSteppers } from './kit.js';

let settingsSeq = 0;
const SETTINGS_EVENT = 'wk-settings-change';
const make = (tag, cls, text) => {
  const el = document.createElement(tag);
  if (cls) el.className = cls;
  if (text != null) el.textContent = text;
  return el;
};
const sameOptions = (a, b) => a.length === b.length && a.every((o, i) => o.value === b[i].value && o.label === b[i].label);

class WkSettings extends HTMLElement {
  constructor() {
    super();
    this.idPrefix = `wk-settings-${++settingsSeq}`;
    this.state = { schema: null, values: {}, errors: [], icons: {}, validate: null };
    this.rowsByKey = new Map();
    this.fieldsByKey = new Map();
    this.tabsByGroup = new Map();
    this.advancedByGroup = new Map();
  }

  connectedCallback() {
    if (!this.built && this.state.schema) this.render();
  }

  get schema() { return this.state.schema; }
  set schema(schema) { this.state.schema = schema || null; this.render(); }

  get values() { return { ...this.state.values }; }
  set values(values) { this.state.values = { ...(values || {}) }; this.sync(); }

  get errors() { return [...this.state.errors]; }
  set errors(errors) { this.state.errors = Array.isArray(errors) ? [...errors] : []; this.sync(); }

  get icons() { return this.state.icons; }
  set icons(icons) { this.state.icons = icons || {}; this.render(); }

  get validate() { return this.state.validate; }
  set validate(fn) { this.state.validate = typeof fn === 'function' ? fn : null; }

  idFor(key, part) { return `${this.idPrefix}-${String(key).replace(/[^\w-]/g, '_')}-${part}`; }

  // Rebuilds everything from the schema. Called when the schema or icons change.
  render() {
    const schema = this.state.schema;
    this.replaceChildren();
    this.rowsByKey.clear(); this.fieldsByKey.clear(); this.tabsByGroup.clear(); this.advancedByGroup.clear();
    this.built = Boolean(schema);
    if (!schema) return;
    const model = settingsModel(schema, this.state.values, this.state.errors);
    for (const group of schema.groups || []) for (const field of group.fields || []) this.fieldsByKey.set(field.key, field);
    if (!model.groups.length) {
      const empty = make('div', 'wk-empty');
      empty.append(make('p', null, 'There are no settings.'));
      this.append(empty);
      return;
    }
    const panes = model.groups.map(g => this.pane(g, model.groups.length === 1));
    if (model.groups.length === 1) {
      this.append(panes[0]);
    } else {
      const tabs = document.createElement('wk-tabs');
      tabs.className = 'wk-settings-window';
      const list = make('div', 'wk-settings-tabs');
      list.setAttribute('role', 'tablist');
      list.setAttribute('aria-label', this.getAttribute('label') || 'Settings');
      model.groups.forEach((g, i) => {
        const tab = make('button', 'wk-settings-tab');
        tab.type = 'button';
        tab.setAttribute('role', 'tab');
        tab.id = this.idFor(g.key, 'tab');
        tab.setAttribute('aria-controls', panes[i].id);
        tab.setAttribute('aria-selected', String(i === 0));
        const icon = this.state.icons[g.key];
        if (icon instanceof Node) tab.append(icon.cloneNode(true));
        else if (typeof icon === 'string' && icon) tab.insertAdjacentHTML('beforeend', icon);
        tab.append(make('span', null, g.label));
        list.append(tab);
        this.tabsByGroup.set(g.key, tab);
      });
      tabs.append(list, ...panes);
      this.append(tabs);
    }
    this.sync();
  }

  pane(group, only) {
    const pane = make('div', 'wk-settings-pane');
    pane.id = this.idFor(group.key, 'pane');
    pane.dataset.group = group.key;
    if (!only) pane.setAttribute('role', 'tabpanel');
    else pane.append(make('h2', 'wk-settings-heading', group.label));
    if (group.rows.length) {
      const box = make('div', 'wk-panel wk-panel--rows');
      for (const row of group.rows) box.append(this.row(row));
      pane.append(box);
    }
    if (group.advanced.length) {
      const toggle = make('button', 'wk-link wk-settings-advanced', 'Advanced');
      toggle.type = 'button';
      const box = make('div', 'wk-panel wk-panel--rows');
      box.id = this.idFor(group.key, 'advanced');
      box.hidden = true;
      toggle.setAttribute('aria-expanded', 'false');
      toggle.setAttribute('aria-controls', box.id);
      toggle.addEventListener('click', () => this.expand(group.key, toggle.getAttribute('aria-expanded') !== 'true'));
      for (const row of group.advanced) box.append(this.row(row));
      pane.append(toggle, box);
      this.advancedByGroup.set(group.key, { toggle, box });
    }
    return pane;
  }

  expand(groupKey, open) {
    const a = this.advancedByGroup.get(groupKey);
    if (!a) return;
    a.toggle.setAttribute('aria-expanded', String(open));
    a.box.hidden = !open;
  }

  row(r) {
    const field = this.fieldsByKey.get(r.key);
    const row = make('div', 'wk-settings-row');
    row.dataset.key = r.key;
    row.dataset.type = r.type;
    const controlId = this.idFor(r.key, 'control');
    const helpId = this.idFor(r.key, 'help');
    const errorId = this.idFor(r.key, 'error');
    const error = make('p', 'wk-error');
    error.id = errorId;
    error.hidden = true;
    let control;
    if (r.type === 'toggle') {
      const label = make('label', 'wk-check wk-check--row');
      const words = make('span', 'wk-settings-label');
      words.append(make('span', null, r.label));
      if (r.description) { const help = make('small', 'wk-settings-help', r.description); help.id = helpId; words.append(help); }
      control = make('input', 'wk-switch');
      control.type = 'checkbox';
      control.setAttribute('role', 'switch');
      label.append(words, control);
      row.append(label);
    } else {
      const words = make('div', 'wk-settings-label');
      const label = make(r.type === 'computed' ? 'span' : 'label', null, r.label);
      label.id = this.idFor(r.key, 'label');
      if (r.type !== 'computed') label.htmlFor = controlId;
      words.append(label);
      if (r.description) { const help = make('span', 'wk-settings-help', r.description); help.id = helpId; words.append(help); }
      row.append(words);
      if (r.type === 'select') {
        control = make('select', 'wk-select');
        row.append(control);
      } else if (r.type === 'number') {
        const stepper = make('span', 'wk-stepper');
        control = make('input', 'wk-field wk-num');
        control.type = 'number';
        control.inputMode = 'numeric';
        if (r.min != null) control.min = String(r.min);
        if (r.max != null) control.max = String(r.max);
        const arrows = make('span', 'wk-stepper-arrows');
        for (const name of ['More', 'Fewer']) { const b = make('button'); b.type = 'button'; b.setAttribute('aria-label', name); arrows.append(b); }
        stepper.append(control, arrows);
        row.append(stepper);
      } else if (r.type === 'computed') {
        control = make('output', 'wk-settings-value');
        control.setAttribute('aria-labelledby', label.id);
        row.append(control);
      } else {
        control = make('input', 'wk-field wk-settings-control');
        control.type = r.type === 'password' ? 'password' : 'text';
        control.autocomplete = 'off';
        control.spellcheck = false;
        if (r.placeholder) control.placeholder = r.placeholder;
        if (r.type === 'password') control.addEventListener('focus', () => { if (control.value === SECRET_MASK) control.select(); });
        row.append(control);
      }
    }
    control.id = controlId;
    if (r.required && r.type !== 'computed' && r.type !== 'toggle') control.setAttribute('aria-required', 'true');
    if (r.type !== 'computed') {
      control.addEventListener('change', () => this.commit(field, r.type === 'toggle' ? control.checked : control.value));
    }
    row.append(error);
    this.rowsByKey.set(r.key, { row, control, error, helpId: r.description ? helpId : null, options: null });
    return row;
  }

  commit(field, raw) {
    const values = this.state.values;
    values[field.key] = readValue(field, raw);
    Object.assign(values, dependentResets(this.state.schema, field.key, values));
    let errors = [];
    if (this.state.validate) {
      errors = this.state.validate(this.state.schema, { ...values }) || [];
      this.state.errors = [...errors];
    } else {
      // The app's errors stay until it sets new ones, except on the field just changed.
      this.state.errors = this.state.errors.filter(e => e.field !== field.key);
    }
    this.sync();
    this.dispatchEvent(new CustomEvent(SETTINGS_EVENT, { bubbles: true, detail: { key: field.key, value: values[field.key], values: { ...values }, errors: [...errors] } }));
  }

  // Brings every control in line with the values and errors, keeping the DOM
  // (and focus) as it is.
  sync() {
    const schema = this.state.schema;
    if (!this.built || !schema) return;
    const model = settingsModel(schema, this.state.values, this.state.errors);
    for (const group of model.groups) {
      for (const r of [...group.rows, ...group.advanced]) {
        const parts = this.rowsByKey.get(r.key);
        if (!parts) continue;
        const { row, control, error } = parts;
        row.hidden = !r.visible;
        if (r.type === 'toggle') control.checked = Boolean(r.value);
        else if (r.type === 'computed') control.textContent = r.text;
        else if (r.type === 'select') {
          const options = [...(r.missing ? [{ value: '', label: r.disabled ? 'None available' : 'Choose…', placeholder: true }] : []), ...r.options];
          if (!parts.options || !sameOptions(parts.options, options)) {
            control.replaceChildren(...options.map(o => {
              const opt = make('option', null, o.label);
              opt.value = o.value;
              if (o.placeholder) { opt.disabled = true; }
              return opt;
            }));
            parts.options = options;
          }
          control.value = r.missing ? '' : String(r.value);
          control.disabled = r.disabled;
        } else if (document.activeElement !== control || control.value === '') {
          control.value = r.text;
        }
        if (r.type === 'number') syncSteppers(row);
        const describedBy = [parts.helpId, r.error ? error.id : null].filter(Boolean);
        if (describedBy.length) control.setAttribute('aria-describedby', describedBy.join(' '));
        else control.removeAttribute('aria-describedby');
        if (r.error) { control.setAttribute('aria-invalid', 'true'); error.textContent = r.error; error.hidden = false; }
        else { control.removeAttribute('aria-invalid'); error.textContent = ''; error.hidden = true; }
      }
      this.tabsByGroup.get(group.key)?.toggleAttribute('data-attention', group.attention);
      if (group.advanced.some(r => r.error)) this.expand(group.key, true);
    }
  }
}

const defineSettings = (name, cls) => { if (!customElements.get(name)) customElements.define(name, cls); };
defineSettings('wk-settings', WkSettings);

export { WkSettings };
