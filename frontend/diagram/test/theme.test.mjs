// Mermaid's theme variables come from the kit's tokens, and every text and
// ground pair in them reaches 4.5:1 (lines and marks 3:1): for the kit's
// light and dark tokens, the example theme, a tinted-grounds theme and
// forced colours. test/browser.mjs checks the same in the page.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { root } from '../scripts/build.mjs';
import { buildTheme, TEXT_PAIRS, MARK_PAIRS, TOKENS, contrast, parseColour, mix, hex } from '../src/theme.js';
import { tokens } from '../../ui/test/css.mjs';

const ui = p => readFileSync(path.join(root, '..', 'ui', p), 'utf8');
const kit = tokens(ui('src/tokens.css'));

// A token reader for one appearance: the kit's values with a theme's on top;
// chart-1 is derived from the accent (THEMING.md), as color-mix() does.
function reader(appearance, themeCss) {
  const theme = themeCss ? tokens(themeCss)[appearance] : new Map();
  const values = new Map([...kit[appearance], ...theme]);
  if (theme.has('--wk-color-accent')) values.set('--wk-color-chart-1', theme.get('--wk-color-accent'));
  return name => {
    const v = values.get(name) ?? kit.light.get(name);
    assert.ok(v != null, `no value for ${name}`);
    return v;
  };
}

const CASES = [
  ['kit light', reader('light')], ['kit dark', reader('dark')], ['kit dark (data-theme)', reader('attrDark')],
  ['example light', reader('light', ui('theme/example.css'))], ['example dark', reader('dark', ui('theme/example.css'))],
  ['warm grounds light', reader('light', ui('test/fixtures/warm-grounds.css'))], ['warm grounds dark', reader('dark', ui('test/fixtures/warm-grounds.css'))],
];

test('every token the theme reads is a kit token', () => {
  for (const name of TOKENS) assert.ok(kit.light.has(name), `${name} is not in ui/src/tokens.css`);
});

test('colours parse from hex, rgb(), rgba() and color(srgb …)', () => {
  assert.deepEqual(parseColour('#fff'), [255, 255, 255, 1]);
  assert.deepEqual(parseColour('rgba(27, 31, 39, .045)'), [27, 31, 39, 0.045]);
  assert.deepEqual(parseColour('rgb(1 2 3 / 0.5)'), [1, 2, 3, 0.5]);
  assert.deepEqual(parseColour('color(srgb 1 0 0.5 / 0.25)').map(Math.round), [255, 0, 128, 0]);
  assert.equal(parseColour('Canvas'), null);
  assert.equal(hex(mix([255, 255, 255, 1], [0, 0, 0, 1], 0.5)), '#808080');
});

for (const [name, get] of CASES) {
  test(`${name}: text pairs reach 4.5:1 and lines 3:1; colours are opaque hex`, () => {
    const { themeVariables: v, darkMode } = buildTheme(get);
    assert.equal(darkMode, name.includes('dark'));
    for (const [k, value] of Object.entries(v)) {
      if (/Color|Bkg|Background|Border|^cScale|^background$|^primary|^secondary|^tertiary|^lineColor$|^classText$/.test(k)) assert.match(String(value), /^#[0-9a-f]{6}$/, `${k} is ${value}`);
    }
    for (const [text, ground] of TEXT_PAIRS) {
      assert.ok(v[text] && v[ground], `${text} or ${ground} is not set`);
      const ratio = contrast(v[text], v[ground]);
      assert.ok(ratio >= 4.5, `${name}: ${text} ${v[text]} on ${ground} ${v[ground]} is ${ratio.toFixed(2)}:1`);
    }
    for (const [mark, ground] of MARK_PAIRS) {
      const ratio = contrast(v[mark], v[ground]);
      assert.ok(ratio >= 3, `${name}: ${mark} ${v[mark]} on ${ground} ${v[ground]} is ${ratio.toFixed(2)}:1`);
    }
  });
}

test('the theme follows the tokens: ground, text, lines, accent, chart series, font', () => {
  const get = reader('light', ui('theme/example.css'));
  const { themeVariables: v, fontFamily, fontSize } = buildTheme(get);
  assert.equal(v.background, get('--wk-color-surface'));
  assert.equal(v.textColor, get('--wk-color-text'));
  assert.equal(v.primaryTextColor, get('--wk-color-text'));
  assert.equal(v.lineColor, get('--wk-color-text-secondary'));
  assert.equal(v.nodeBorder, get('--wk-color-field-border'));
  assert.equal(v.activationBorderColor, get('--wk-color-accent'));
  assert.equal(v.todayLineColor, get('--wk-color-accent'));
  assert.equal(v.activeTaskBorderColor, '#1f6b55', 'the app theme\'s accent');
  // Categorical fills are the chart series tinted into the ground.
  const ground = parseColour(get('--wk-color-surface'));
  for (let i = 1; i <= 5; i++) assert.equal(v[`cScale${i - 1}`], hex(mix(parseColour(get(`--wk-color-chart-${i}`)), ground, 0.16)));
  assert.equal(fontFamily, get('--wk-font-family'));
  assert.match(fontFamily, /^system-ui,/);
  assert.equal(fontSize, 13);
  assert.equal(v.fontSize, '13px');
});

test('forced colours: every ground is Canvas and every line and text CanvasText', () => {
  const { themeVariables: v } = buildTheme(reader('light'), { forced: { canvas: 'rgb(0, 0, 0)', text: 'rgb(255, 255, 0)' } });
  for (const [text, ground] of TEXT_PAIRS) {
    // The sequence number is the one text on a mark (a line-coloured circle): Canvas on CanvasText.
    const inverse = ground === 'signalColor';
    assert.equal(v[text], inverse ? '#000000' : '#ffff00', text);
    assert.equal(v[ground], inverse ? '#ffff00' : '#000000', ground);
  }
  for (const [mark] of MARK_PAIRS) assert.equal(v[mark], '#ffff00', mark);
});

test('a missing token says so', () => {
  assert.throws(() => buildTheme(() => ''), /--wk-color-surface is not a colour.*@wails-kit\/ui's stylesheet/);
});
