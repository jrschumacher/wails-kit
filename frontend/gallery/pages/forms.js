// Forms: fields, choices, steppers, ranges and the settings window.
import { glyphs, stepper } from './shared.js';

// A sample settings schema, in the shape settings.Service.GetSchema() returns.
const SECRET_MASK = '••••••••';
const settingsSchema = {
  groups: [
    {
      key: 'general', label: 'General', fields: [
        { key: 'general.name', type: 'text', label: 'Display name', description: 'Shown to people you share with.', placeholder: 'Your name', validation: { required: true, maxLen: 40 } },
        { key: 'general.launch', type: 'toggle', label: 'Open at login', description: 'Starts in the background when you sign in.', default: true },
        { key: 'general.updates', type: 'select', label: 'Check for updates', default: 'daily', options: [{ label: 'Daily', value: 'daily' }, { label: 'Weekly', value: 'weekly' }, { label: 'Never', value: 'never' }] },
        { key: 'general.folder', type: 'text', label: 'Projects folder', description: 'Checked before it applies.', placeholder: '~/Projects', check: { pending: 'Checking folder…' } },
        { key: 'general.copies', type: 'number', label: 'Copies to keep', default: 2, validation: { min: 1, max: 8 } },
        { key: 'general.config', type: 'computed', label: 'Settings file' },
      ],
    },
    {
      key: 'provider', label: 'Provider', fields: [
        { key: 'provider.name', type: 'select', label: 'Provider', default: 'alpha', options: [{ label: 'Alpha', value: 'alpha' }, { label: 'Local', value: 'local' }] },
        { key: 'provider.key', type: 'password', label: 'API key', description: 'Kept in the system keychain.', condition: { field: 'provider.name', equals: ['alpha'] } },
        { key: 'provider.port', type: 'number', label: 'Port', default: 8080, condition: { field: 'provider.name', equals: ['local'] }, validation: { min: 1024, max: 65535 } },
        { key: 'provider.endpoint', type: 'text', label: 'Endpoint', placeholder: 'https://api.example.com', advanced: true },
      ],
    },
  ],
};

export default [
  {
    id: 'field', title: 'Field', group: 'forms', card: 'SearchField',
    intro: 'A label, the field, then help or an error under it. <code>aria-invalid</code> draws the error state; <code>wk-search</code> adds the glyph.',
    examples: [{
      title: 'States',
      html: `<div class="g-grid">
  <div><label class="wk-label" for="g-f1">Name</label><input id="g-f1" class="wk-field g-wide" value="Notes"><p class="wk-help">Help text under a field.</p></div>
  <div><label class="wk-label" for="g-f2">Placeholder</label><input id="g-f2" class="wk-field g-wide" placeholder="name@example.com"></div>
  <div><label class="wk-label" for="g-f3">Invalid</label><input id="g-f3" class="wk-field g-wide" value="not an address" aria-invalid="true" aria-describedby="g-f3-e"><p class="wk-error" id="g-f3-e">Enter an email address.</p></div>
  <div><label class="wk-label" for="g-f4">Disabled</label><input id="g-f4" class="wk-field g-wide" value="Locked" disabled></div>
  <div><label class="wk-label" for="g-f5">Number</label><input id="g-f5" class="wk-field g-narrow wk-num" type="number" value="2" min="1" max="8"></div>
  <div><label class="wk-label" for="g-f6">Search</label><div class="wk-search g-wide"><input id="g-f6" class="wk-field" type="search" placeholder="Search"></div></div>
  <div class="g-span"><label class="wk-label" for="g-f7">Text area</label><textarea id="g-f7" class="wk-field g-wide">Several lines
of text.</textarea></div>
</div>`,
    }],
  },
  {
    id: 'check', title: 'Checkbox, radio and switch', group: 'forms', card: 'CheckRow',
    intro: 'Native inputs with the kit\'s face, in a <code>wk-check</code> label. A switch is a checkbox with <code>role="switch"</code>.',
    examples: [{
      title: 'Checkboxes',
      html: `<div class="g-row">
  <label class="wk-check"><input class="wk-checkbox" type="checkbox"> Off</label>
  <label class="wk-check"><input class="wk-checkbox" type="checkbox" checked> On</label>
  <label class="wk-check"><input class="wk-checkbox" type="checkbox" id="g-mixed"> Mixed</label>
  <label class="wk-check"><input class="wk-checkbox" type="checkbox" disabled> Disabled</label>
  <label class="wk-check"><input class="wk-checkbox" type="checkbox" checked disabled> Disabled on</label>
</div>`,
      after(el) {
        // Mixed is a property, not an attribute.
        el.querySelector('#g-mixed').indeterminate = true;
      },
    }, {
      title: 'With a help line',
      note: 'wk-check--top aligns the box with the first line; wk-check-text stacks the words and the help.',
      html: `<label class="wk-check wk-check--top"><input class="wk-checkbox" type="checkbox" checked><span class="wk-check-text"><span>Send crash reports</span><span class="wk-caption">Only the stack and the app's version; never your files.</span></span></label>`,
    }, {
      title: 'Radio buttons',
      html: `<div class="g-row" role="radiogroup" aria-label="Radio buttons">
  <label class="wk-check"><input class="wk-radio" type="radio" name="g-radio"> Off</label>
  <label class="wk-check"><input class="wk-radio" type="radio" name="g-radio" checked> On</label>
  <label class="wk-check"><input class="wk-radio" type="radio" name="g-radio-disabled" disabled> Disabled</label>
  <label class="wk-check"><input class="wk-radio" type="radio" name="g-radio-disabled" checked disabled> Disabled on</label>
</div>`,
    }, {
      title: 'Switches',
      html: `<div class="g-row">
  <label class="wk-check"><input class="wk-switch" type="checkbox" role="switch"> Off</label>
  <label class="wk-check"><input class="wk-switch" type="checkbox" role="switch" checked> On</label>
  <label class="wk-check"><input class="wk-switch" type="checkbox" role="switch" disabled> Disabled</label>
  <label class="wk-check"><input class="wk-switch" type="checkbox" role="switch" checked disabled> Disabled on</label>
</div>`,
    }],
  },
  {
    id: 'select', title: 'Select', group: 'forms',
    intro: 'A native <code>&lt;select&gt;</code>: its menu is the system\'s.',
    examples: [{
      title: 'Enabled and disabled',
      html: `<div class="g-row">
  <select class="wk-select" aria-label="Range"><option>Last 14 days</option><option>Last 30 days</option></select>
  <select class="wk-select" aria-label="Disabled select" disabled><option>Disabled</option></select>
</div>`,
    }],
  },
  {
    id: 'filters', title: 'Filter buttons', group: 'forms', card: 'FilterButtons',
    intro: 'Toggle buttons (<code>aria-pressed</code>) that filter a list, each with its count. The app toggles them; this page does.',
    examples: [{
      title: 'Pressed, not pressed and disabled',
      html: `<div class="wk-filters" role="group" aria-label="Filter" id="g-filters">
  <button class="wk-filter" type="button" aria-pressed="true">All <span class="wk-filter-count">12</span></button>
  <button class="wk-filter" type="button" aria-pressed="false">Open <span class="wk-filter-count">3</span></button>
  <button class="wk-filter" type="button" aria-pressed="false">Closed <span class="wk-filter-count">1</span></button>
  <button class="wk-filter" type="button" aria-pressed="false" disabled>Archived</button>
</div>`,
      after(el) {
        el.addEventListener('click', e => {
          const b = e.target.closest('.wk-filter');
          if (b && !b.disabled) b.setAttribute('aria-pressed', String(b.getAttribute('aria-pressed') !== 'true'));
        });
      },
    }],
  },
  {
    id: 'segmented', title: 'Segmented control', group: 'forms', card: 'SegmentedControl',
    intro: 'Radio buttons drawn as one control: arrow keys move the choice.',
    examples: [{
      title: 'With a disabled choice',
      html: `<div class="wk-segmented" role="radiogroup" aria-label="Period">
  <label><input type="radio" name="g-period" value="day" checked><span>Day</span></label>
  <label><input type="radio" name="g-period" value="week"><span>Week</span></label>
  <label><input type="radio" name="g-period" value="month"><span>Month</span></label>
  <label><input type="radio" name="g-period" value="year" disabled><span>Year</span></label>
</div>`,
    }],
  },
  {
    id: 'stepper', title: 'Stepper', group: 'forms', card: 'Stepper',
    intro: 'The arrows step the value between min and max (kit.js), fire input and change, and disable at a limit. <code>syncSteppers()</code> redraws them after the app sets a value.',
    examples: [{
      title: 'States',
      note: 'type="number" (its own spinner hidden); at the limit; a text field; invalid.',
      html: `<div class="g-row">
  <label class="wk-label" for="g-stepper">Jobs at once</label>${stepper('g-stepper', 'Jobs at once', 2)}
  ${stepper('g-stepper-max', 'At the limit', 8, { more: false })}
  ${stepper('g-stepper-text', 'A text field', 3, { text: true })}
  ${stepper('', 'Invalid', 0, { fewer: false, invalid: true })}
</div>`,
    }],
  },
  {
    id: 'range', title: 'Range', group: 'forms', card: 'Range',
    intro: 'The app sets <code>--wk-range-value</code> to the filled share through the CSSOM (a CSP blocks a <code>style</code> attribute) and updates it on input, with the value in words beside the slider.',
    examples: [{
      title: 'Values and disabled',
      html: `<div class="g-ranges" id="g-ranges">
  <span>First share</span><input class="wk-range" id="g-range" type="range" min="0" max="100" step="5" value="60" aria-label="First share"><output for="g-range">60%</output>
  <span>Second share</span><input class="wk-range" type="range" min="0" max="100" step="5" value="30" aria-label="Second share"><output>30%</output>
  <span>Disabled</span><input class="wk-range" type="range" min="0" max="100" value="25" aria-label="Disabled example" disabled><output>25%</output>
</div>`,
      after(el) {
        const share = r => {
          const value = `${Math.round((r.value - r.min) / (r.max - r.min) * 100)}%`;
          r.style.setProperty('--wk-range-value', value);
          r.nextElementSibling.textContent = value;
        };
        el.querySelectorAll('.wk-range').forEach(share);
        el.addEventListener('input', e => { if (e.target.matches('.wk-range')) share(e.target); });
      },
    }],
  },
  {
    id: 'settings-rows', title: 'Settings group and rows', group: 'forms',
    intro: 'The rows of a settings pane: a label (with help) and its control. <code>wk-check--row</code> makes the whole row one label.',
    examples: [{
      title: 'A group',
      html: `<div class="g-measure">
  <h3 class="wk-settings-heading">General</h3>
  <div class="wk-settings-group">
    <div class="wk-settings-row"><label class="wk-settings-label" for="g-s1">Open at login<span class="wk-settings-help">Starts in the background when you sign in.</span></label><input id="g-s1" class="wk-switch" type="checkbox" role="switch" checked></div>
    <div class="wk-settings-row"><label class="wk-settings-label" for="g-s1b">Show a notice when a download fails</label><input id="g-s1b" class="wk-checkbox" type="checkbox"></div>
    <div class="wk-settings-row"><label class="wk-check wk-check--row"><span class="wk-settings-label">Copies<small class="wk-settings-help">The whole row is the hit area.</small></span>${stepper('g-s2', 'Copies', 2)}</label></div>
    <div class="wk-settings-row"><label class="wk-settings-label" for="g-s3">Check for updates</label><select id="g-s3" class="wk-select"><option>Daily</option><option>Weekly</option></select></div>
    <div class="wk-settings-row"><span class="wk-settings-label">Reset all settings<span class="wk-settings-help">Puts every setting back to its default.</span></span><button class="wk-button wk-button--destructive" type="button">Reset…</button></div>
  </div>
  <p class="wk-settings-note">A note under a group.</p>
</div>`,
    }],
  },
  {
    id: 'settings-tabs', title: 'Settings tabs', group: 'forms', card: 'SettingsTabs',
    intro: 'The icon tabs at the top of a settings window. <code>data-attention</code> marks a pane with a field to review.',
    examples: [{
      title: 'Selected, and one that needs attention',
      html: `<div class="wk-settings-tabs g-measure" role="tablist" aria-label="Settings">
  <button class="wk-settings-tab" role="tab" type="button" aria-selected="true">${glyphs.general}General</button>
  <button class="wk-settings-tab" role="tab" type="button" aria-selected="false" tabindex="-1" data-attention aria-describedby="g-attention-why">${glyphs.sharing}Sharing</button>
  <button class="wk-settings-tab" role="tab" type="button" aria-selected="false" tabindex="-1">${glyphs.about}About</button>
</div>
<p class="wk-caption" id="g-attention-why">Sharing carries data-attention: a field there needs review.</p>`,
    }],
  },
  {
    id: 'settings-window', title: 'Settings window', group: 'forms',
    intro: '<code>&lt;wk-settings&gt;</code> drawn from a settings schema: every field type, a condition, advanced fields, a computed value and a check. A stand-in for SetValues rejects an empty name and a port under 1024; a stand-in for CheckValue accepts a folder starting with / or ~/ after 600 ms.',
    examples: [{
      title: 'The window',
      html: '<div class="g-settings-window"><wk-settings id="g-settings" label="Settings"></wk-settings></div>',
      after(el) {
        const settings = el.querySelector('wk-settings');
        settings.schema = settingsSchema;
        settings.icons = { general: glyphs.general, provider: glyphs.sharing };
        settings.check = (key, values) => new Promise(resolve => setTimeout(() => {
          const path = String(values[key] ?? '');
          resolve(path === '' || path.startsWith('/') || path.startsWith('~/') ? [] : [{ field: key, message: 'There is no folder at this path. Choose a folder that exists.', code: 'check' }]);
        }, 600));
        settings.values = { 'general.name': 'Sam', 'general.folder': '~/Projects', 'general.launch': true, 'general.updates': 'weekly', 'general.copies': 2, 'general.config': '~/.config/example/settings.json', 'provider.name': 'alpha', 'provider.key': SECRET_MASK };
        settings.addEventListener('wk-settings-change', e => {
          if (e.detail.checked) return;
          const v = e.detail.values;
          const errors = [];
          if (!String(v['general.name'] ?? '').trim()) errors.push({ field: 'general.name', message: 'Display name is required', code: 'required' });
          if (v['provider.name'] === 'local' && typeof v['provider.port'] === 'number' && v['provider.port'] < 1024) errors.push({ field: 'provider.port', message: 'Port must be at least 1024', code: 'min' });
          settings.errors = errors;
        });
      },
    }],
  },
];
