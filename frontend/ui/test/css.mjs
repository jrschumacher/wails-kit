// CSS helpers for the static style checks (test/tokens.test.mjs): colour
// literals, ::backdrop overrides and banned literals. Reading a sheet's rules
// and tokens, and WCAG contrast, live in the shipped theme check
// (theme/check.mjs) and are re-exported here.
import { rules, tokens, TOKEN_SELECTORS, isTokenSelector, over, contrast } from '../theme/check.mjs';
export { rules, tokens, TOKEN_SELECTORS, isTokenSelector, over, contrast };

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

// The literals a component rule may not use: system colours only inside a
// forced-colors block, every other literal nowhere.
export const banned = (r, value) => colourLiterals(value).filter(l => !(r.forced && l.startsWith('system:')));

// The selectors of every rule in a sheet, one per comma-separated part, with
// the at-rules each sits in: [{ selector, atRules, decls }].
export function selectorsOf(css) {
  return rules(css).flatMap(r => splitSelectors(r.selector).map(selector => ({ selector, atRules: r.atRules, decls: r.decls, forced: r.forced })));
}
// A selector list split at top-level commas (not those inside :where() etc).
export function splitSelectors(list) {
  const out = [];
  let depth = 0, start = 0;
  for (let i = 0; i < list.length; i++) {
    if (list[i] === '(') depth++;
    else if (list[i] === ')') depth--;
    else if (list[i] === ',' && depth === 0) { out.push(list.slice(start, i).trim()); start = i + 1; }
  }
  out.push(list.slice(start).trim());
  return out;
}
