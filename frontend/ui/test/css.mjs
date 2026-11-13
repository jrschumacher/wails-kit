// CSS helpers for the static style checks (test/tokens.test.mjs): colour
// literals, the rules of a sheet, the --wk-* tokens it sets, and WCAG contrast.

// Colour literals in a declaration value: hex, a colour function, a CSS
// named colour, or a system colour. currentColor, transparent and inherit are
// not literals; var(--wk-…) is a token.
const NAMED = new Set(('aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue blueviolet brown '
  + 'burlywood cadetblue chartreuse chocolate coral cornflowerblue cornsilk crimson cyan darkblue darkcyan darkgoldenrod '
  + 'darkgray darkgreen darkgrey darkkhaki darkmagenta darkolivegreen darkorange darkorchid darkred darksalmon darkseagreen '
  + 'darkslateblue darkslategray darkslategrey darkturquoise darkviolet deeppink deepskyblue dimgray dimgrey dodgerblue '
  + 'firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray green greenyellow grey honeydew '
  + 'hotpink indianred indigo ivory khaki lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral lightcyan '
  + 'lightgoldenrodyellow lightgray lightgreen lightgrey lightpink lightsalmon lightseagreen lightskyblue lightslategray '
  + 'lightslategrey lightsteelblue lightyellow lime limegreen linen magenta maroon mediumaquamarine mediumblue mediumorchid '
  + 'mediumpurple mediumseagreen mediumslateblue mediumspringgreen mediumturquoise mediumvioletred midnightblue mintcream '
  + 'mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid palegoldenrod palegreen '
  + 'paleturquoise palevioletred papayawhip peachpuff peru pink plum powderblue purple rebeccapurple red rosybrown '
  + 'royalblue saddlebrown salmon sandybrown seagreen seashell sienna silver skyblue slateblue slategray slategrey snow '
  + 'springgreen steelblue tan teal thistle tomato turquoise violet wheat white whitesmoke yellow yellowgreen').split(' '));
const SYSTEM = new Set(('accentcolor accentcolortext activetext buttonborder buttonface buttontext canvas canvastext field '
  + 'fieldtext graytext highlight highlighttext linktext mark marktext selecteditem selecteditemtext visitedtext').split(' '));
export function colourLiterals(value) {
  const v = value.replace(/"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'/g, '').replace(/var\(\s*--[\w-]+\s*\)/g, '');
  const found = [...v.matchAll(/#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color)\(/gi)].map(m => m[0]);
  for (const [word] of v.matchAll(/(?<![\w-])[a-z][\w-]*/gi)) {
    const w = word.toLowerCase();
    if (NAMED.has(w)) found.push(word);
    else if (SYSTEM.has(w)) found.push(`system:${word}`);
  }
  return found;
}
export const COLOUR = { test: value => colourLiterals(value).length > 0 };
const DARK = /^@media\s*\(\s*prefers-color-scheme\s*:\s*dark\s*\)$/;
const FORCED = /^@media\s*\(\s*forced-colors\s*:\s*active\s*\)$/;

// Every rule with declarations: { selector, decls: [[prop, value]], dark, atRules }.
export function rules(css) {
  css = css.replace(/\/\*[\s\S]*?\*\//g, '');
  const out = [];
  const stack = [];
  let start = 0;
  for (let i = 0; i < css.length; i++) {
    const c = css[i];
    if (c === '{') {
      stack.push(css.slice(start, i).trim().replace(/\s+/g, ' '));
      start = i + 1;
    } else if (c === '}') {
      const body = css.slice(start, i);
      const selector = stack.pop();
      if (body.includes(':')) {
        const decls = body.split(';').map(d => d.trim()).filter(Boolean).map(d => {
          const at = d.indexOf(':');
          return [d.slice(0, at).trim(), d.slice(at + 1).trim()];
        }).filter(([p]) => p && !p.startsWith('@'));
        out.push({ selector, decls, atRules: [...stack], dark: stack.some(s => DARK.test(s)), forced: stack.some(s => FORCED.test(s)), attrDark: ATTR_DARK.test(selector) });
      }
      start = i + 1;
    } else if (c === ';' && stack.length === 0) {
      start = i + 1; // top-level @import
    }
  }
  return out;
}

// The theme: light on :root; dark under the media query on
// :root:not([data-theme="light"]), and again on :root[data-theme="dark"], so
// the attribute wins over the system setting either way.
const ATTR_DARK = /^:root\[data-theme="dark"\]/;
export const TOKEN_SELECTORS = { light: ':root', dark: ':root:not([data-theme="light"])', attrDark: ':root[data-theme="dark"]' };
export const isTokenSelector = selector => Object.values(TOKEN_SELECTORS).includes(selector);

// The --wk-* tokens a sheet sets: light (:root), dark (the media query) and
// attrDark (data-theme="dark"). misplaced lists token blocks that follow
// neither pattern, such as a bare :root inside the media query.
export function tokens(css) {
  const light = new Map(), dark = new Map(), attrDark = new Map(), misplaced = [];
  for (const r of rules(css)) {
    if (!isTokenSelector(r.selector)) continue;
    const map = !r.dark && r.selector === TOKEN_SELECTORS.light ? light
      : r.dark && r.selector === TOKEN_SELECTORS.dark ? dark
      : !r.dark && r.selector === TOKEN_SELECTORS.attrDark ? attrDark : null;
    if (!map) { misplaced.push(`${r.atRules.join(' ')} ${r.selector}`.trim()); continue; }
    for (const [p, v] of r.decls) if (p.startsWith('--wk-')) map.set(p, v);
  }
  return { light, dark, attrDark, misplaced };
}

// A selector without the theme prefix of a dark rule, and which appearance
// the rule is for: 'light', 'dark' (media query) or 'attrDark' (attribute).
export function themed(r) {
  const plain = r.selector.replace(/^:root:not\(\[data-theme="light"\]\)\s+/, '').replace(/^:root\[data-theme="dark"\]\s+/, '');
  return { plain, theme: r.dark ? 'dark' : r.attrDark ? 'attrDark' : 'light' };
}

// Every ::backdrop rule in a sheet keeps a literal colour per appearance:
// returns the problems (a missing dark override, a token in a backdrop).
export function backdropProblems(css) {
  const seen = { light: [], dark: [], attrDark: [] };
  const problems = [];
  for (const r of rules(css)) {
    if (!r.selector.includes('::backdrop')) continue;
    const { plain, theme } = themed(r);
    seen[theme].push(plain);
    if (theme === 'dark' && !r.selector.startsWith(TOKEN_SELECTORS.dark + ' ')) problems.push(`${r.selector}: a dark ::backdrop needs the ${TOKEN_SELECTORS.dark} prefix`);
    for (const [, value] of r.decls) if (value.includes('var(')) problems.push(`${r.selector} uses a token; ::backdrop needs a literal colour`);
  }
  const sorted = a => [...a].sort().join(', ');
  if (sorted(seen.dark) !== sorted(seen.light)) problems.push(`::backdrop rules [${sorted(seen.light)}] have media-query dark overrides [${sorted(seen.dark)}]`);
  if (sorted(seen.attrDark) !== sorted(seen.light)) problems.push(`::backdrop rules [${sorted(seen.light)}] have data-theme="dark" overrides [${sorted(seen.attrDark)}]`);
  return problems;
}

// WCAG 2 contrast of two #rrggbb colours, and a colour laid over another.
const rgb = c => {
  const m = c.replace(/\s+/g, '').match(/^#([0-9a-f]{6})$|^rgba\((\d+),(\d+),(\d+),([\d.]+)\)$/i);
  if (!m) throw new Error(`not a colour: ${c}`);
  return m[1] ? [[0, 2, 4].map(i => parseInt(m[1].slice(i, i + 2), 16)), 1] : [[+m[2], +m[3], +m[4]], +m[5]];
};
export function over(top, ground) {
  const [t, a] = rgb(top), [g] = rgb(ground);
  return '#' + t.map((c, i) => Math.round(c * a + g[i] * (1 - a)).toString(16).padStart(2, '0')).join('');
}
export function contrast(a, b) {
  const lum = c => { const [v] = rgb(c); const [r, g, bl] = v.map(x => { x /= 255; return x <= .03928 ? x / 12.92 : ((x + .055) / 1.055) ** 2.4; }); return .2126 * r + .7152 * g + .0722 * bl; };
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
  return (x + .05) / (y + .05);
}

// The literals a component rule may not use: system colours only inside a
// forced-colors block, every other literal nowhere.
export const banned = (r, value) => colourLiterals(value).filter(l => !(r.forced && l.startsWith('system:')));
