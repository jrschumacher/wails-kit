// The settings window's review page: a <wk-settings> drawn from a sample
// schema in the shape settings.Service.GetSchema() returns, with every field
// type, a condition, dynamic options, advanced fields and a computed value.
// A stand-in for the app's SetValues answers each change: it rejects an
// empty name and a port under 1024, as the Go service's validation would.
const SECRET_MASK = '••••••••';
const glyph = path => `<svg viewBox="0 0 22 22" aria-hidden="true"><g fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">${path}</g></svg>`;

export const schema = {
  groups: [
    {
      key: 'general', label: 'General', fields: [
        { key: 'general.name', type: 'text', label: 'Display name', description: 'Shown to people you share with.', placeholder: 'Your name', validation: { required: true, maxLen: 40 } },
        { key: 'general.launch', type: 'toggle', label: 'Open at login', description: 'Starts in the background when you sign in.', default: true },
        { key: 'general.updates', type: 'select', label: 'Check for updates', default: 'daily', options: [{ label: 'Daily', value: 'daily' }, { label: 'Weekly', value: 'weekly' }, { label: 'Never', value: 'never' }] },
        { key: 'general.copies', type: 'number', label: 'Copies to keep', default: 2, validation: { min: 1, max: 8 } },
        { key: 'general.config', type: 'computed', label: 'Settings file' },
      ],
    },
    {
      key: 'provider', label: 'Provider', fields: [
        { key: 'provider.name', type: 'select', label: 'Provider', default: 'alpha', options: [{ label: 'Alpha', value: 'alpha' }, { label: 'Beta', value: 'beta' }, { label: 'Local', value: 'local' }] },
        { key: 'provider.model', type: 'select', label: 'Model', dynamicOptions: { dependsOn: 'provider.name', options: {
          alpha: [{ label: 'Alpha Small', value: 'a-small' }, { label: 'Alpha Large', value: 'a-large' }],
          beta: [{ label: 'Beta One', value: 'b-1' }],
        } } },
        { key: 'provider.key', type: 'password', label: 'API key', description: 'Kept in the system keychain.', condition: { field: 'provider.name', equals: ['alpha', 'beta'] } },
        { key: 'provider.port', type: 'number', label: 'Port', description: 'Where the local server listens.', default: 8080, condition: { field: 'provider.name', equals: ['local'] }, validation: { min: 1024, max: 65535 } },
        { key: 'provider.endpoint', type: 'text', label: 'Endpoint', placeholder: 'https://api.example.com', advanced: true },
        { key: 'provider.debug', type: 'toggle', label: 'Log requests', advanced: true },
      ],
    },
  ],
};
const icons = {
  general: glyph('<circle cx="11" cy="11" r="3"/><path d="M11 2.5v2.5M11 17v2.5M2.5 11H5M17 11h2.5M5 5l1.8 1.8M15.2 15.2 17 17M5 17l1.8-1.8M15.2 6.8 17 5"/>'),
  provider: glyph('<rect x="4" y="4.5" width="14" height="10" rx="1.3"/><path d="M2 17.5h18"/>'),
};

// The stand-in for SetValues: the errors the Go service would return.
function setValues(values) {
  const errors = [];
  if (!String(values['general.name'] ?? '').trim()) errors.push({ field: 'general.name', message: 'Display name is required', code: 'required' });
  const port = values['provider.port'];
  if (values['provider.name'] === 'local' && typeof port === 'number' && port < 1024) errors.push({ field: 'provider.port', message: 'Port must be at least 1024', code: 'min' });
  return errors;
}

const host = document.getElementById('settings-demo');
host.innerHTML = `<header class="demo-head"><div><h1 class="wk-title">Settings window</h1>
<p class="wk-caption">&lt;wk-settings&gt; drawn from a sample settings schema. Changes go to a stand-in for SetValues, which rejects an empty name and a port under 1024.</p></div>
<nav class="demo-row"><a class="wk-link" href="index.html">Kit demo</a></nav></header>
<div class="demo-settings-window"><wk-settings id="demo-settings" label="Settings"></wk-settings></div>
<pre id="demo-log" class="demo-log" aria-live="polite"></pre>`;
const el = document.getElementById('demo-settings');
el.schema = schema;
el.icons = icons;
el.values = { 'general.name': 'Sam', 'general.launch': true, 'general.updates': 'weekly', 'general.copies': 2, 'general.config': '~/.config/example/settings.json', 'provider.name': 'alpha', 'provider.model': 'a-large', 'provider.key': SECRET_MASK };
window.settingsEvents = [];
el.addEventListener('wk-settings-change', e => {
  window.settingsEvents.push(JSON.parse(JSON.stringify(e.detail)));
  el.errors = setValues(e.detail.values);
  document.getElementById('demo-log').textContent = `wk-settings-change ${e.detail.key} = ${JSON.stringify(e.detail.value)}\n` + document.getElementById('demo-log').textContent.split('\n').slice(0, 5).join('\n');
});
