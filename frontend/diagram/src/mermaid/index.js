// The lazy half of @wails-kit/diagram: Mermaid, configured for the kit, and
// one function that turns source text into a sanitized SVG element.
// dist/mermaid.js is this module with Mermaid bundled; dist/diagram.js loads
// it the first time a diagram renders. scripts/build.mjs drops the diagram
// types and layouts the kit does not offer, and patches Mermaid's styling
// through csp.js so it draws under a strict Content-Security-Policy.
import mermaid from 'mermaid';
import { setStyle, takeSheet } from './csp.js';

/** Mermaid's diagram ids for the types the kit offers, and the kit's name for each. */
export const TYPES = {
  'flowchart-v2': 'flowchart', flowchart: 'flowchart',
  sequence: 'sequence',
  stateDiagram: 'state', state: 'state',
  classDiagram: 'class', class: 'class',
  er: 'er',
  gantt: 'gantt',
};

// What a diagram's own front matter or %%{init}%% directive may not change:
// Mermaid's own secure keys, and everything that sets the look, the layout
// or HTML labels, which the kit holds.
const SECURE = ['secure', 'securityLevel', 'startOnLoad', 'maxTextSize', 'suppressErrorRendering', 'maxEdges',
  'theme', 'themeVariables', 'themeCSS', 'darkMode', 'fontFamily', 'fontSize', 'look', 'layout', 'htmlLabels', 'handDrawnSeed', 'deterministicIds'];

/** A source the kit will not draw, or one Mermaid could not parse. */
export class DiagramError extends Error {
  constructor(message, { line = null, kind = 'syntax', cause } = {}) {
    super(message, cause ? { cause } : undefined);
    this.name = 'DiagramError';
    this.line = line;
    this.kind = kind;
  }
}

function configure(theme) {
  const font = { fontFamily: theme.fontFamily };
  const size = theme.fontSize;
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'strict',
    suppressErrorRendering: true,
    logLevel: 5,
    theme: 'base',
    darkMode: !!theme.darkMode,
    themeVariables: theme.themeVariables,
    fontFamily: theme.fontFamily,
    fontSize: size,
    look: 'classic',
    layout: 'dagre',
    htmlLabels: false,
    markdownAutoWrap: true,
    secure: SECURE,
    flowchart: { htmlLabels: false, useMaxWidth: false },
    sequence: {
      useMaxWidth: false, actorFontFamily: theme.fontFamily, noteFontFamily: theme.fontFamily, messageFontFamily: theme.fontFamily,
      actorFontSize: size, noteFontSize: size, messageFontSize: size,
    },
    class: { htmlLabels: false, useMaxWidth: false },
    state: { useMaxWidth: false },
    er: { useMaxWidth: false, ...font },
    // Gantt spreads over its container's width; a fixed width keeps it legible when fitted.
    gantt: { useMaxWidth: false, useWidth: 800, fontSize: size, sectionFontSize: size },
  });
}

// The line of a parse error: Jison's hash, else "on line N" in the message.
function lineOf(err) {
  const loc = err?.hash?.loc;
  if (loc && Number.isInteger(loc.first_line)) return loc.first_line;
  if (Number.isInteger(err?.hash?.line)) return err.hash.line + 1;
  const m = String(err?.message ?? '').match(/\bline:?\s*(\d+)/i);
  return m ? Number(m[1]) : null;
}

const SVG_NS = 'http://www.w3.org/2000/svg';

// After DOMPurify: links and references that leave the document go (a
// click in a desktop app's web view would navigate it), and style
// attributes come back through the CSSOM.
function finish(svg) {
  for (const a of [...svg.querySelectorAll('a')]) a.replaceWith(...a.childNodes);
  for (const el of svg.querySelectorAll('*')) {
    for (const attr of [...el.attributes]) {
      const name = attr.name.toLowerCase();
      if ((name === 'href' || name === 'xlink:href' || name === 'src') && !attr.value.trim().startsWith('#')) el.removeAttributeNode(attr);
      else if (name.startsWith('on')) el.removeAttributeNode(attr);
    }
  }
  for (const el of svg.querySelectorAll('image, script, style, iframe, object, embed')) el.remove();
  for (const el of [svg, ...svg.querySelectorAll('[data-wk-style]')]) {
    const style = el.getAttribute('data-wk-style');
    if (style == null) continue;
    el.removeAttribute('data-wk-style');
    setStyle(el, style);
  }
}

let counter = 0;
let queue = Promise.resolve();

/**
 * Renders `source` with `theme` ({ themeVariables, fontFamily, fontSize,
 * darkMode }). Resolves to { type, svg, sheet }: the kit's name for the
 * diagram type, a detached <svg> and its stylesheet (adopted by the
 * document; call sheet.remove() when the SVG goes). Rejects with a
 * DiagramError. Renders run one at a time: Mermaid's configuration is
 * global.
 */
export function render(source, theme) {
  const run = queue.then(() => renderNow(String(source ?? ''), theme));
  queue = run.catch(() => {});
  return run;
}

async function renderNow(source, theme) {
  configure(theme);
  let id;
  try {
    id = mermaid.detectType(source);
  } catch (err) {
    throw new DiagramError(source.trim() ? 'No diagram type found. Start with flowchart, sequenceDiagram, stateDiagram-v2, classDiagram, erDiagram or gantt.' : 'The diagram is empty.', { kind: 'unsupported', cause: err });
  }
  const type = TYPES[id];
  if (!type) throw new DiagramError(`"${id}" diagrams are not supported. Use a flowchart, sequence, state, class, entity-relationship or Gantt diagram.`, { kind: 'unsupported' });
  const domId = `wk-diagram-${++counter}`;
  let result;
  try {
    result = await mermaid.render(domId, source);
  } catch (err) {
    takeSheet(domId)?.remove();
    document.getElementById(domId)?.remove();
    document.getElementById(`d${domId}`)?.remove();
    throw new DiagramError(String(err?.message ?? err).trim() || 'The diagram could not be drawn.', { line: lineOf(err), cause: err });
  }
  const template = document.createElement('template');
  template.innerHTML = result.svg;
  const parsed = template.content.querySelector('svg');
  if (!parsed || parsed.namespaceURI !== SVG_NS) { takeSheet(domId)?.remove(); throw new DiagramError('Mermaid returned no SVG.'); }
  // Into this document before its styles go back through the CSSOM.
  const svg = document.adoptNode(parsed);
  finish(svg);
  return { type, svg, sheet: takeSheet(domId) };
}
