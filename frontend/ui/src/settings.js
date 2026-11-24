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
// from SetValues), icons ({ groupKey: svg markup or a Node }), validate (an
// optional (schema, values) => errors, such as @wails-kit/settings' validate)
// and check (an optional (key, values) => Promise<errors>, the app's
// settings.Service.CheckValue binding, for fields with a back-end check).
// Data out through one bubbling event, wk-settings-change, fired when a value
// applies: detail { key, value, values, errors, checked }. values holds every
// field; errors is what validate returned ([] without it); checked is true
// when the value passed a back-end check, which has already saved it. The app
// saves the rest (SetValues) and sets errors from the reply.
//
// Validation timing follows BEHAVIOUR.md's "Forms and validation": a field is
// checked when it commits (change), and once it shows an error it is
// re-checked on every keystroke, so the error clears as soon as it's fixed. A
// field with a back-end check applies only once the check passes: its words
// ("Checking token…") show beside it after 400 ms and stay at least 500 ms;
// once it shows an error, the check runs again 500 ms after typing stops.
// Typing is never blocked. An empty required field shows its error from the
// moment the window opens, and the window's Notice counts the fields that need
// attention ("2 settings need attention.").
import { SECRET_MASK, settingsModel, readValue, dependentResets } from './settings-model.js';
import { syncSteppers } from './kit.js';
import { timings } from './motion.js';

let settingsSeq = 0;
const SETTINGS_EVENT = 'wk-settings-change';
const CODE_CHECK = 'check';
const CODE_REQUIRED = 'required';
const TYPED = new Set(['text', 'password', 'number']);
// A label mid-sentence: its first letter lowercased unless its first word has
// other capitals ("API key", "GitHub token"), as the Go settings package does.
const inSentence = label => {
  const first = label.split(/\s/)[0];
  return /^\p{Lu}/u.test(first) && !/\p{Lu}/u.test(first.slice(1)) ? label[0].toLowerCase() + label.slice(1) : label;
};
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
    this.state = { schema: null, values: {}, errors: [], icons: {}, validate: null, check: null };
    // drafts: key -> the text a control holds that hasn't applied (being
    // re-checked as typed, or waiting on or failed by its back-end check).
    // live: keys re-checked as typed, from when they show an error until a
    // value applies with none. checks: key -> the back-end check running.
    this.drafts = new Map();
    this.live = new Set();
    this.checks = new Map();
    this.rechecks = new Map();
    this.noticeTimer = null;
    this.noticeCount = 0;
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
  set values(values) {
    const next = { ...(values || {}) };
    // A field whose value the app changes drops what the window kept for it:
    // the person's unapplied text and a check's failure, both about another value.
    for (const key of new Set([...Object.keys(next), ...Object.keys(this.state.values)])) {
      if (Object.is(next[key], this.state.values[key])) continue;
      this.drafts.delete(key);
      this.live.delete(key);
      this.state.errors = this.state.errors.filter(e => !(e.field === key && e.code === CODE_CHECK));
    }
    this.state.values = next;
    this.sync();
  }

  get errors() { return [...this.state.errors]; }
  set errors(errors) {
    const next = Array.isArray(errors) ? [...errors] : [];
    // A failed back-end check stays on its field until that field commits or
    // is checked again, unless these errors say something new about it.
    const failed = this.state.errors.filter(e => e.code === CODE_CHECK && !next.some(n => n.field === e.field));
    this.state.errors = [...next, ...failed];
    this.sync();
  }

  get icons() { return this.state.icons; }
  set icons(icons) { this.state.icons = icons || {}; this.render(); }

  get validate() { return this.state.validate; }
  set validate(fn) { this.state.validate = typeof fn === 'function' ? fn : null; }

  get check() { return this.state.check; }
  set check(fn) { this.state.check = typeof fn === 'function' ? fn : null; }

  /** Shows a field: selects its tab, opens Advanced if it's there, and focuses its control. False when the field isn't shown. */
  reveal(key) {
    const parts = this.rowsByKey.get(key);
    if (!parts || parts.row.hidden) return false;
    const pane = parts.row.closest('.wk-settings-pane');
    const tabs = this.querySelector(':scope > wk-tabs');
    if (tabs && pane) tabs.selectedIndex = [...this.tabsByGroup.values()].findIndex(t => t.getAttribute('aria-controls') === pane.id);
    if (parts.row.closest('[hidden]') && pane) this.expand(pane.dataset.group, true);
    parts.control.focus();
    return true;
  }

  idFor(key, part) { return `${this.idPrefix}-${String(key).replace(/[^\w-]/g, '_')}-${part}`; }

  // Rebuilds everything from the schema. Called when the schema or icons change.
  render() {
    const schema = this.state.schema;
    this.replaceChildren();
    this.rowsByKey.clear(); this.fieldsByKey.clear(); this.tabsByGroup.clear(); this.advancedByGroup.clear();
    for (const run of this.checks.values()) clearTimeout(run.timer);
    for (const timer of this.rechecks.values()) clearTimeout(timer);
    this.checks.clear(); this.rechecks.clear(); this.drafts.clear(); this.live.clear();
    clearTimeout(this.noticeTimer);
    this.noticeCount = 0;
    // The window's Notice: a polite region, empty until a field needs attention.
    this.notice = make('div', 'wk-settings-status');
    this.notice.setAttribute('role', 'status');
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
      panes[0].querySelector(':scope > .wk-settings-heading').after(this.notice);
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
      tabs.append(list, this.notice, ...panes);
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
    if (TYPED.has(r.type)) control.addEventListener('input', () => this.typed(field, control.value));
    // A back-end check's words: a polite region, empty until the check has
    // run for timings.indicatorDelay, then a small spinner and the words.
    let pending = null;
    if (field?.check) {
      pending = make('p', 'wk-settings-pending');
      pending.id = this.idFor(r.key, 'pending');
      pending.setAttribute('role', 'status');
      row.append(pending);
    }
    row.append(error);
    this.rowsByKey.set(r.key, { row, control, error, pending, helpId: r.description ? helpId : null, options: null });
    return row;
  }

  // A control committed a value (change). A field with a back-end check runs
  // it; any other applies at once.
  commit(field, raw) {
    const key = field.key;
    clearTimeout(this.rechecks.get(key));
    this.rechecks.delete(key);
    // Every value of a checked field goes through the check, an empty one
    // included: the back end saves it (and drops a recorded failure) without
    // checking it.
    if (field.check && this.state.check) { this.runCheck(field, raw); return; }
    this.drafts.delete(key);
    this.apply(field, readValue(field, raw));
  }

  apply(field, value, { checked = false } = {}) {
    const key = field.key;
    const values = this.state.values;
    values[key] = value;
    Object.assign(values, dependentResets(this.state.schema, key, values));
    let errors = [];
    // A back-end check's failure stays on its field until that field commits or is checked again.
    const failed = this.state.errors.filter(e => e.code === CODE_CHECK && e.field !== key);
    if (this.state.validate) {
      errors = this.state.validate(this.state.schema, { ...values }) || [];
      this.state.errors = [...errors, ...failed.filter(e => !errors.some(n => n.field === e.field))];
    } else {
      // The app's errors stay until it sets new ones, except on the field just changed.
      this.state.errors = this.state.errors.filter(e => e.field !== key);
    }
    if (!this.shownErrors().has(key)) this.live.delete(key);
    this.sync();
    this.dispatchEvent(new CustomEvent(SETTINGS_EVENT, { bubbles: true, detail: { key, value: values[key], values: { ...values }, errors: [...errors], checked } }));
  }

  // The person typed in a field. Once it shows an error, re-check it now
  // with the draft, and its back-end check 500 ms after typing stops.
  typed(field, raw) {
    const key = field.key;
    if (this.shownErrors().has(key)) this.live.add(key);
    if (this.rowsByKey.get(key)?.pending?.hasAttribute('data-outcome')) this.showPending(key, null);
    if (!this.live.has(key)) return;
    this.drafts.set(key, raw);
    const own = this.ownErrors(field, raw);
    const kept = own.length ? [] : this.state.errors.filter(e => e.field === key && e.code === CODE_CHECK);
    const appKept = this.state.validate ? [] : this.state.errors.filter(e => e.field === key && e.code !== CODE_CHECK && e.code !== CODE_REQUIRED);
    this.state.errors = [...this.state.errors.filter(e => e.field !== key), ...own, ...appKept, ...kept];
    // The field and its tab follow each keystroke; the window's Notice waits
    // until typing stops, so its count doesn't flicker or announce per key.
    this.sync({ notice: false });
    clearTimeout(this.noticeTimer);
    this.noticeTimer = setTimeout(() => this.updateNotice(), timings.recheckDelay);
    clearTimeout(this.rechecks.get(key));
    this.rechecks.delete(key);
    // A checked field goes to the back end once typing stops: a valid draft
    // to be checked, an invalid one so a check of an earlier value can't save it.
    if (field.check && this.state.check) {
      this.rechecks.set(key, setTimeout(() => { this.rechecks.delete(key); this.runCheck(field, raw); }, timings.recheckDelay));
    }
  }

  // The field's own errors with this draft: validate's, else none (the model
  // marks an empty required field itself).
  ownErrors(field, raw) {
    if (!this.state.validate) return [];
    const values = { ...this.state.values, [field.key]: readValue(field, raw) };
    return (this.state.validate(this.state.schema, values) || []).filter(e => e.field === field.key);
  }

  // Runs a field's back-end check on raw. The value applies only if it
  // passes; until then the control keeps the person's text and typing goes on.
  runCheck(field, raw) {
    const key = field.key;
    const value = readValue(field, raw);
    const values = { ...this.state.values, [key]: value };
    Object.assign(values, dependentResets(this.state.schema, key, values));
    this.drafts.set(key, raw);
    const prior = this.checks.get(key);
    if (prior) clearTimeout(prior.timer);
    const own = this.ownErrors(field, raw);
    const label = field.label || key;
    if (prior?.shownAt == null) this.showPending(key, null);
    if (own.length) {
      this.checks.delete(key);
      this.showPending(key, null);
      this.state.errors = [...this.state.errors.filter(e => e.field !== key), ...own];
      this.live.add(key);
      this.sync();
      // Still tell the back end: the value it may be checking was abandoned,
      // so an older check that passes mustn't save it. CheckValue validates
      // the field first, overtakes that check, and saves nothing; its reply
      // repeats the errors shown, so it is ignored.
      Promise.resolve().then(() => this.state.check(key, values)).catch(() => {});
      return;
    }
    const run = { raw, value, timer: null, shownAt: null };
    this.checks.set(key, run);
    // Keep the indicator a run already shows; else show it after 400 ms.
    if (prior?.shownAt != null) run.shownAt = prior.shownAt;
    else run.timer = setTimeout(() => { run.shownAt = Date.now(); this.showPending(key, field.check.pending || 'Checking…'); }, timings.indicatorDelay);
    this.sync();
    Promise.resolve()
      .then(() => this.state.check(key, values))
      .then(errs => (Array.isArray(errs) ? errs : []), () => [{ field: key, message: `Couldn't check your ${inSentence(label)}. Edit it to check again.`, code: CODE_CHECK }])
      .then(errs => this.settle(field, run, errs));
  }

  settle(field, run, errs) {
    const key = field.key;
    if (this.checks.get(key) !== run) return;
    clearTimeout(run.timer);
    const finish = () => {
      if (this.checks.get(key) !== run) return;
      this.checks.delete(key);
      const label = field.label || key;
      // No errors means the value was saved (CheckValue's contract). Errors
      // only on other fields mean it wasn't: they show where they belong, and
      // this field says so.
      let mine = errs.filter(e => e.field === key);
      const others = errs.filter(e => e.field !== key);
      if (others.length && !mine.length) mine = [{ field: key, message: `Your ${inSentence(label)} wasn't saved. Fix the other settings, then enter it again.`, code: CODE_CHECK }];
      if (mine.length) {
        mine = mine.map(e => ({ ...e, code: e.code || CODE_CHECK }));
        const replaced = new Set(errs.map(e => e.field));
        this.state.errors = [...this.state.errors.filter(e => !replaced.has(e.field)), ...others, ...mine];
        this.live.add(key);
        this.showPending(key, null, mine[0].message);
        return;
      }
      if (this.drafts.get(key) === run.raw) this.drafts.delete(key);
      this.showPending(key, null, `${label} checked`, false);
      this.apply(field, run.value, { checked: true });
    };
    // Once shown, the words stay at least 500 ms; the result waits and both change in one step.
    const wait = run.shownAt == null ? 0 : run.shownAt + timings.indicatorMin - Date.now();
    if (wait > 0) run.timer = setTimeout(finish, wait);
    else finish();
  }

  // The check's status region: the pending words with a small spinner, or the
  // outcome in words for screen readers (visually hidden: the field shows
  // its error or its value), or nothing.
  showPending(key, words, outcome, sync = true) {
    const parts = this.rowsByKey.get(key);
    if (!parts?.pending) return;
    if (!words && !outcome && !parts.pending.firstChild) return;
    parts.pending.toggleAttribute('data-outcome', Boolean(outcome));
    if (outcome) {
      parts.pending.replaceChildren(make('span', null, outcome));
      delete parts.row.dataset.pending;
    } else if (words) {
      const spinner = make('span', 'wk-spinner');
      spinner.setAttribute('aria-hidden', 'true');
      parts.pending.replaceChildren(spinner, make('span', null, words));
      parts.row.dataset.pending = '';
    } else {
      parts.pending.replaceChildren();
      delete parts.row.dataset.pending;
    }
    if (sync) this.sync();
  }

  // The values the window shows: the applied ones with the drafts typed over them.
  shownValues() {
    const out = { ...this.state.values };
    for (const [key, raw] of this.drafts) {
      const field = this.fieldsByKey.get(key);
      if (field) out[key] = readValue(field, raw);
    }
    return out;
  }

  shownErrors() {
    const out = new Set();
    const model = settingsModel(this.state.schema, this.shownValues(), this.state.errors);
    for (const g of model.groups) for (const r of [...g.rows, ...g.advanced]) if (r.error) out.add(r.key);
    return out;
  }

  // Brings every control in line with the values and errors, keeping the DOM
  // (and focus) as it is.
  sync({ notice = true } = {}) {
    const schema = this.state.schema;
    if (!this.built || !schema) return;
    const model = settingsModel(schema, this.shownValues(), this.state.errors);
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
          control.value = this.drafts.has(r.key) ? this.drafts.get(r.key) : r.text;
        }
        if (r.type === 'number') syncSteppers(row);
        const describedBy = [parts.helpId, parts.row.hasAttribute('data-pending') ? parts.pending.id : null, r.error ? error.id : null].filter(Boolean);
        if (describedBy.length) control.setAttribute('aria-describedby', describedBy.join(' '));
        else control.removeAttribute('aria-describedby');
        if (r.error) { control.setAttribute('aria-invalid', 'true'); error.textContent = r.error; error.hidden = false; }
        else { control.removeAttribute('aria-invalid'); error.textContent = ''; error.hidden = true; }
      }
      this.tabsByGroup.get(group.key)?.toggleAttribute('data-attention', group.attention);
      if (group.advanced.some(r => r.error)) this.expand(group.key, true);
    }
    if (notice) this.updateNotice(model);
  }

  // The window's Notice: the count of fields that need attention. Its words
  // change, and so are announced, only when the count does.
  updateNotice(model = settingsModel(this.state.schema, this.shownValues(), this.state.errors)) {
    clearTimeout(this.noticeTimer);
    if (!this.notice) return;
    const count = model.attention;
    if (count === this.noticeCount && (count === 0) === !this.notice.firstChild) return;
    this.noticeCount = count;
    if (!count) { this.notice.replaceChildren(); return; }
    const words = count === 1 ? '1 setting needs attention.' : `${count} settings need attention.`;
    const shown = this.notice.querySelector('b');
    if (shown) { shown.textContent = words; return; }
    const box = make('div', 'wk-notice wk-notice--inline');
    box.dataset.tone = 'attention';
    const body = make('div', 'wk-notice-body');
    body.append(make('b', null, words));
    box.append(body);
    this.notice.append(box);
  }
}

const defineSettings = (name, cls) => { if (!customElements.get(name)) customElements.define(name, cls); };
defineSettings('wk-settings', WkSettings);

export { WkSettings };
