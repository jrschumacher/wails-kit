// Mermaid's theme variables, built from the kit's --wk-* tokens. Pure: the
// caller passes read(name), which returns a token's value (in the browser,
// index.js reads computed values; the tests read tokens.css). Every colour
// handed to Mermaid is opaque #rrggbb, composited over the diagram's ground,
// so each text and ground pair below has one contrast ratio to check.

/** The tokens the theme reads. */
export const TOKENS = [
  '--wk-color-surface', '--wk-color-text', '--wk-color-text-secondary', '--wk-color-hairline-strong',
  '--wk-color-field-border', '--wk-color-fill', '--wk-color-fill-strong', '--wk-color-accent',
  '--wk-color-chart-1', '--wk-color-chart-2', '--wk-color-chart-3', '--wk-color-chart-4', '--wk-color-chart-5',
  '--wk-font-family', '--wk-font-size-base',
];

/** [r, g, b, a] (0–255, alpha 0–1) from #hex, rgb(), rgba() or color(srgb …); null if unreadable. */
export function parseColour(value) {
  const s = String(value ?? '').trim().toLowerCase();
  let m = s.match(/^#([0-9a-f]{3,8})$/);
  if (m) {
    let h = m[1];
    if (h.length === 3 || h.length === 4) h = [...h].map(c => c + c).join('');
    if (h.length !== 6 && h.length !== 8) return null;
    const n = [0, 2, 4, 6].map(i => parseInt(h.slice(i, i + 2) || 'ff', 16));
    return [n[0], n[1], n[2], n[3] / 255];
  }
  m = s.match(/^rgba?\(([^)]+)\)$/);
  if (m) {
    const p = m[1].split(/[\s,/]+/).filter(Boolean).map(parseFloat);
    if (p.length < 3 || p.some(Number.isNaN)) return null;
    return [p[0], p[1], p[2], p[3] ?? 1];
  }
  m = s.match(/^color\(srgb\s+([^)]+)\)$/);
  if (m) {
    const p = m[1].split(/[\s/]+/).filter(Boolean).map(parseFloat);
    if (p.length < 3 || p.some(Number.isNaN)) return null;
    return [p[0] * 255, p[1] * 255, p[2] * 255, p[3] ?? 1];
  }
  return null;
}

/** `top` laid over the opaque `ground`. */
export const over = (top, ground) => [0, 1, 2].map(i => top[i] * top[3] + ground[i] * (1 - top[3])).concat(1);
/** `a` at `share` (0–1) mixed into `b`. */
export const mix = (a, b, share) => [0, 1, 2].map(i => a[i] * share + b[i] * (1 - share)).concat(1);
export const hex = c => `#${c.slice(0, 3).map(x => Math.round(Math.min(255, Math.max(0, x))).toString(16).padStart(2, '0')).join('')}`;

function luminance(c) {
  const [r, g, b] = c.slice(0, 3).map(x => { x /= 255; return x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4; });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}
/** WCAG contrast ratio of two opaque colours (parsed or strings). */
export function contrast(a, b) {
  const [x, y] = [a, b].map(c => luminance(typeof c === 'string' ? parseColour(c) : c)).sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
}

/**
 * Text and ground pairs in the theme variables: [text, ground]. Each reaches
 * 4.5:1 (test/theme.test.mjs and the browser checks hold it to that).
 */
export const TEXT_PAIRS = [
  ['textColor', 'background'], ['titleColor', 'background'],
  ['primaryTextColor', 'primaryColor'], ['nodeTextColor', 'mainBkg'],
  ['secondaryTextColor', 'secondaryColor'], ['tertiaryTextColor', 'tertiaryColor'],
  ['textColor', 'clusterBkg'], ['textColor', 'edgeLabelBackground'],
  // Sequence
  ['actorTextColor', 'actorBkg'], ['signalTextColor', 'background'], ['labelTextColor', 'labelBoxBkgColor'],
  ['loopTextColor', 'background'], ['noteTextColor', 'noteBkgColor'], ['sequenceNumberColor', 'signalColor'],
  // State
  ['stateLabelColor', 'stateBkg'], ['transitionLabelColor', 'labelBackgroundColor'], ['textColor', 'compositeBackground'],
  ['textColor', 'compositeTitleBackground'], ['textColor', 'altBackground'],
  // Class and entity-relationship
  ['classText', 'mainBkg'], ['textColor', 'attributeBackgroundColorOdd'], ['textColor', 'attributeBackgroundColorEven'],
  ['relationLabelColor', 'relationLabelBackground'],
  // Gantt
  ['taskTextColor', 'taskBkgColor'], ['taskTextLightColor', 'taskBkgColor'], ['taskTextDarkColor', 'taskBkgColor'],
  ['taskTextColor', 'activeTaskBkgColor'], ['taskTextDarkColor', 'activeTaskBkgColor'],
  ['taskTextColor', 'doneTaskBkgColor'], ['taskTextDarkColor', 'doneTaskBkgColor'],
  ['taskTextColor', 'critBkgColor'], ['taskTextDarkColor', 'critBkgColor'],
  ['taskTextOutsideColor', 'background'], ['taskTextOutsideColor', 'sectionBkgColor'], ['taskTextOutsideColor', 'altSectionBkgColor'],
  ['titleColor', 'sectionBkgColor'], ['titleColor', 'altSectionBkgColor'],
  ['cScaleLabel0', 'cScale0'], ['cScaleLabel1', 'cScale1'], ['cScaleLabel2', 'cScale2'], ['cScaleLabel3', 'cScale3'], ['cScaleLabel4', 'cScale4'],
];

/**
 * Lines and marks against the ground they are drawn on: [mark, ground].
 * Each reaches 3:1 (WCAG 1.4.11).
 */
export const MARK_PAIRS = [
  ['lineColor', 'background'], ['nodeBorder', 'background'], ['primaryBorderColor', 'background'],
  ['actorBorder', 'background'], ['actorLineColor', 'background'], ['signalColor', 'background'],
  ['activationBorderColor', 'background'], ['transitionColor', 'background'], ['relationColor', 'background'],
  ['todayLineColor', 'background'], ['specialStateColor', 'background'],
];

function read(get, name) {
  const c = parseColour(get(name));
  if (!c) throw new Error(`@wails-kit/diagram: ${name} is not a colour ("${get(name)}"); is @wails-kit/ui's stylesheet on the page?`);
  return c;
}

/**
 * The theme for Mermaid from the tokens: { themeVariables, fontFamily,
 * fontSize, darkMode }. `get(name)` returns a token's value. With
 * `forced: { canvas, text }` (the system's Canvas and CanvasText, in forced
 * colours) every ground is Canvas and every line and text CanvasText.
 */
export function buildTheme(get, { forced = null } = {}) {
  const fontFamily = String(get('--wk-font-family') || 'system-ui, sans-serif').trim();
  const fontSize = parseFloat(get('--wk-font-size-base')) || 13;
  const ground = over(read(get, '--wk-color-surface'), [255, 255, 255, 1]);
  const darkMode = luminance(ground) < 0.4;
  const on = name => over(read(get, name), ground);
  let c;
  if (forced) {
    const bg = over(parseColour(forced.canvas) ?? [255, 255, 255, 1], [255, 255, 255, 1]);
    const fg = over(parseColour(forced.text) ?? [0, 0, 0, 1], bg);
    c = { ground: bg, text: fg, secondary: fg, line: fg, border: fg, hairline: fg, node: bg, tint: () => bg, accent: fg, accentTint: bg, fill: bg };
  } else {
    // Categorical tints: a chart colour mixed into the ground, light enough
    // (or dark enough) for body text on it.
    const share = darkMode ? 0.3 : 0.16;
    c = {
      ground, text: on('--wk-color-text'), secondary: on('--wk-color-text-secondary'),
      line: on('--wk-color-text-secondary'), border: on('--wk-color-field-border'), hairline: on('--wk-color-hairline-strong'),
      node: on('--wk-color-fill-strong'), fill: on('--wk-color-fill'),
      tint: n => mix(on(`--wk-color-chart-${n}`), ground, share),
      accent: on('--wk-color-accent'), accentTint: mix(on('--wk-color-accent'), ground, share),
    };
  }
  const h = hex;
  const text = h(c.text), ground_ = h(c.ground), node = h(c.node), fill = h(c.fill), line = h(c.line), border = h(c.border);
  const v = {
    darkMode, fontFamily, fontSize: `${fontSize}px`,
    background: ground_, mainBkg: node, nodeBorder: border, nodeTextColor: text, textColor: text, titleColor: text,
    primaryColor: node, primaryTextColor: text, primaryBorderColor: border,
    secondaryColor: h(c.tint(2)), secondaryTextColor: text, secondaryBorderColor: border,
    tertiaryColor: h(c.tint(3)), tertiaryTextColor: text, tertiaryBorderColor: border,
    lineColor: line, defaultLinkColor: line, arrowheadColor: line,
    clusterBkg: fill, clusterBorder: h(c.hairline), edgeLabelBackground: ground_, labelBackground: ground_,
    errorBkgColor: ground_, errorTextColor: text,
    // Sequence
    actorBkg: node, actorBorder: border, actorTextColor: text, actorLineColor: line,
    signalColor: line, signalTextColor: text, labelBoxBkgColor: node, labelBoxBorderColor: border, labelTextColor: text,
    loopTextColor: text, noteBkgColor: h(c.tint(4)), noteBorderColor: border, noteTextColor: text,
    activationBkgColor: h(c.accentTint), activationBorderColor: h(c.accent), sequenceNumberColor: ground_,
    // State
    stateBkg: node, stateLabelColor: text, labelColor: text, labelBackgroundColor: ground_, compositeBackground: ground_,
    compositeTitleBackground: fill, compositeBorder: border, altBackground: fill, transitionColor: line, transitionLabelColor: text,
    specialStateColor: text, innerEndBackground: text, stateBorder: border,
    // Class and entity-relationship
    classText: text, attributeBackgroundColorOdd: ground_, attributeBackgroundColorEven: fill,
    relationColor: line, relationLabelColor: text, relationLabelBackground: ground_,
    // Gantt: the chart series for tasks, the accent for what is active now
    sectionBkgColor: fill, sectionBkgColor2: fill, altSectionBkgColor: ground_, gridColor: h(c.hairline),
    taskBkgColor: h(c.tint(3)), taskBorderColor: border, taskTextColor: text, taskTextLightColor: text, taskTextDarkColor: text,
    taskTextOutsideColor: text, taskTextClickableColor: text,
    activeTaskBkgColor: h(c.accentTint), activeTaskBorderColor: h(c.accent),
    doneTaskBkgColor: node, doneTaskBorderColor: border, critBkgColor: h(c.tint(5)), critBorderColor: border,
    todayLineColor: h(c.accent), excludeBkgColor: fill, vertLineColor: line,
  };
  for (let i = 0; i < 12; i++) {
    v[`cScale${i}`] = h(c.tint((i % 5) + 1));
    v[`cScaleLabel${i}`] = text;
    v[`cScalePeer${i}`] = border;
  }
  return { themeVariables: v, fontFamily, fontSize, darkMode };
}
