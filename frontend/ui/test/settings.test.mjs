// The settings window's rules (src/settings-model.js), on schemas in the shape
// settings.Service.GetSchema() returns.
import test from 'node:test';
import assert from 'node:assert/strict';
import { SECRET_MASK, conditionMet, resolveOptions, fieldValue, readValue, displayValue, dependentResets, errorsByField, settingsModel } from '../src/settings-model.js';

const schema = {
  groups: [
    { key: 'general', label: 'General', fields: [
      { key: 'name', type: 'text', label: 'Name', placeholder: 'You', validation: { required: true } },
      { key: 'launch', type: 'toggle', label: 'Open at login', default: true },
      { key: 'copies', type: 'number', label: 'Copies', validation: { min: 1, max: 8 } },
      { key: 'path', type: 'computed', label: 'File' },
    ] },
    { key: 'provider', label: 'Provider', fields: [
      { key: 'provider', type: 'select', label: 'Provider', default: 'a', options: [{ label: 'A', value: 'a' }, { label: 'B', value: 'b' }, { label: 'Local', value: 'local' }] },
      { key: 'model', type: 'select', label: 'Model', dynamicOptions: { dependsOn: 'provider', options: { a: [{ label: 'A1', value: 'a1' }, { label: 'A2', value: 'a2' }], b: [{ label: 'B1', value: 'b1' }] } } },
      { key: 'key', type: 'password', label: 'Key', condition: { field: 'provider', equals: ['a', 'b'] } },
      { key: 'endpoint', type: 'text', label: 'Endpoint', advanced: true },
    ] },
  ],
};

test('the secret mask is the one the Go settings service sends', () => {
  assert.equal(SECRET_MASK, '•'.repeat(8));
});

test('conditions and dynamic options behave as @wails-kit/settings does', () => {
  assert.equal(conditionMet(undefined, {}), true);
  assert.equal(conditionMet({ field: 'p', equals: ['a', 'b'] }, { p: 'b' }), true);
  assert.equal(conditionMet({ field: 'p', equals: ['a'] }, { p: 'c' }), false);
  assert.equal(conditionMet({ field: 'p', equals: ['true'] }, { p: true }), true, 'compares as strings');
  assert.equal(conditionMet({ field: 'p', equals: [''] }, {}), true, 'a missing value is ""');
  const model = schema.groups[1].fields[1];
  assert.deepEqual(resolveOptions(model, { provider: 'b' }), [{ label: 'B1', value: 'b1' }]);
  assert.deepEqual(resolveOptions(model, { provider: 'local' }), []);
  assert.deepEqual(resolveOptions(schema.groups[1].fields[0], {}).length, 3);
  assert.deepEqual(resolveOptions({ key: 'x', type: 'select', label: 'X' }, {}), []);
});

test('values fall back to the default, then to a type\'s empty value', () => {
  const [name, launch, copies] = schema.groups[0].fields;
  assert.equal(fieldValue(name, {}), '');
  assert.equal(fieldValue(launch, {}), true);
  assert.equal(fieldValue(launch, { launch: false }), false);
  assert.equal(fieldValue(copies, {}), null);
  assert.equal(fieldValue({ key: 't', type: 'toggle', label: 'T' }, {}), false);
});

test('a control\'s value is read as the schema\'s type', () => {
  const t = type => ({ key: 'k', type, label: 'K' });
  assert.equal(readValue(t('toggle'), 1), true);
  assert.equal(readValue(t('toggle'), false), false);
  assert.equal(readValue(t('number'), '4'), 4);
  assert.equal(readValue(t('number'), ' 2.5 '), 2.5);
  assert.equal(readValue(t('number'), ''), null);
  assert.equal(readValue(t('number'), 'x'), null);
  assert.equal(readValue(t('text'), 42), '42');
  assert.equal(readValue(t('password'), null), '');
  assert.equal(displayValue(t('toggle'), true), 'On');
  assert.equal(displayValue(t('number'), null), '');
  assert.equal(displayValue(t('computed'), { a: 1 }), '{"a":1}');
});

test('a dependent select moves to its first option when its value is no longer offered', () => {
  assert.deepEqual(dependentResets(schema, 'provider', { provider: 'b', model: 'a2' }), { model: 'b1' });
  assert.deepEqual(dependentResets(schema, 'provider', { provider: 'a', model: 'a2' }), {});
  assert.deepEqual(dependentResets(schema, 'provider', { provider: 'local', model: 'a2' }), { model: '' });
  assert.deepEqual(dependentResets(schema, 'name', { provider: 'b', model: 'a2' }), {}, 'only selects that follow the changed key');
});

test('errors: the first message per field; null is no errors', () => {
  assert.deepEqual([...errorsByField(null)], []);
  const m = errorsByField([{ field: 'a', message: 'first', code: 'required' }, { field: 'a', message: 'second', code: 'min' }, { field: 'b', code: 'pattern' }]);
  assert.equal(m.get('a'), 'first');
  assert.equal(m.get('b'), 'pattern');
});

test('the model: groups, rows in order, advanced apart, visibility, options and errors', () => {
  const values = { name: 'Sam', provider: 'local', model: 'a1', path: '/tmp/x.json' };
  const errors = [{ field: 'name', message: 'Name is too short', code: 'min_length' }, { field: 'key', message: 'hidden', code: 'required' }];
  const { groups } = settingsModel(schema, values, errors);
  assert.deepEqual(groups.map(g => [g.key, g.label, g.attention]), [['general', 'General', true], ['provider', 'Provider', false]]);
  assert.deepEqual(groups[0].rows.map(r => r.key), ['name', 'launch', 'copies', 'path']);
  assert.deepEqual(groups[1].rows.map(r => r.key), ['provider', 'model', 'key']);
  assert.deepEqual(groups[1].advanced.map(r => r.key), ['endpoint']);
  const row = key => [...groups[0].rows, ...groups[1].rows, ...groups[1].advanced].find(r => r.key === key);
  assert.equal(row('name').error, 'Name is too short');
  assert.equal(row('name').required, true);
  assert.equal(row('name').placeholder, 'You');
  assert.equal(row('key').visible, false);
  assert.equal(row('key').error, null, 'a hidden field shows no error, and does not mark its group');
  assert.equal(row('launch').value, true);
  assert.deepEqual([row('copies').min, row('copies').max], [1, 8]);
  assert.equal(row('path').text, '/tmp/x.json');
  assert.deepEqual(row('model').options, []);
  assert.equal(row('model').disabled, true);
  assert.equal(row('model').missing, true);
  const a = settingsModel(schema, { provider: 'a', model: 'a2' }).groups[1].rows;
  assert.equal(a[1].missing, false);
  assert.equal(a[1].disabled, false);
  assert.equal(a[2].visible, true);
});

test('an empty or missing schema has no groups', () => {
  assert.deepEqual(settingsModel(null).groups, []);
  assert.deepEqual(settingsModel({ groups: [] }).groups, []);
});
