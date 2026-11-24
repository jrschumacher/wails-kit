// The rich-text editor's document model: a ProseMirror schema holding only
// the features the editor offers (paragraphs and line breaks always; the
// rest per `features`). What the schema lacks cannot get in: pasted HTML and
// loaded content are cut down to it. No DOM, so node tests it.
import { Schema } from 'prosemirror-model';
import { safeUrl, decodeEntities } from '../shared/url.js';

/** Every feature, in toolbar order. */
export const FEATURES = ['bold', 'italic', 'code', 'link', 'heading', 'bulletList', 'orderedList', 'quote', 'codeBlock', 'rule'];

/** The features a `features` option asks for: an array of names (unknown ones throw), or all. */
export function featureSet(features) {
  if (features == null) return new Set(FEATURES);
  if (!Array.isArray(features)) throw new TypeError('wkRichTextEditor: features must be an array of feature names');
  for (const f of features) if (!FEATURES.includes(f)) throw new TypeError(`wkRichTextEditor: unknown feature ${JSON.stringify(f)} (one of ${FEATURES.join(', ')})`);
  return new Set(features);
}

// A code block's language: a short info word, so it is safe in a fence and
// an attribute.
export function languageOf(value) {
  const word = String(value ?? '').trim().split(/\s+/)[0];
  return /^[\w+#.-]{1,32}$/.test(word) ? word : null;
}

/**
 * A link target as the editor stores it, or null when it is not an http,
 * https or mailto URL (safeUrl). It keeps the text as written (after
 * character references are decoded), so Markdown round-trips, unless that
 * text cannot sit in a Markdown link (spaces, angle brackets, backslashes,
 * unbalanced parentheses): then the URL parser's normalised form.
 */
export function linkTarget(href) {
  if (typeof href !== 'string') return null;
  const safe = safeUrl(href);
  if (!safe) return null;
  const written = decodeEntities(href).trim();
  const plain = /^[^\s<>\\]+$/.test(written) && balanced(written) && safeUrl(written) === safe;
  return plain ? written : safe;
}

function balanced(text) {
  let open = 0;
  for (const c of text) {
    if (c === '(') open++;
    else if (c === ')' && --open < 0) return false;
  }
  return open === 0;
}

const headingLevel = n => (n <= 1 ? 1 : 2);
// Inline styles are read from the attribute, as written: under a strict CSP
// the pasted document's style attributes are not applied (element.style is
// empty), and the editor renames them to data-wk-style before parsing
// (index.js, html.js) so the browser logs no violation.
const styleOf = dom => String(dom.getAttribute('data-wk-style') || dom.getAttribute('style') || '').toLowerCase();
const isBold = dom => /(?:^|;)\s*font-weight\s*:\s*(?:bold|bolder|[6-9]00)\b/.test(styleOf(dom));
const isNotBold = dom => /(?:^|;)\s*font-weight\s*:\s*(?:normal|[1-4]00)\b/.test(styleOf(dom));
const isItalic = dom => /(?:^|;)\s*font-style\s*:\s*(?:italic|oblique)\b/.test(styleOf(dom));

/** The schema for a feature set (featureSet()). */
export function buildSchema(features = new Set(FEATURES)) {
  const has = name => features.has(name);
  const lists = has('bulletList') || has('orderedList');
  const nodes = {
    doc: { content: 'block+' },
    paragraph: { content: 'inline*', group: 'block', parseDOM: [{ tag: 'p' }], toDOM: () => ['p', 0] },
  };
  if (has('heading')) {
    nodes.heading = {
      attrs: { level: { default: 1 } },
      // A Markdown heading is one line: no line breaks.
      content: 'text*',
      group: 'block',
      defining: true,
      parseDOM: [1, 2, 3, 4, 5, 6].map(n => ({ tag: `h${n}`, attrs: { level: headingLevel(n) } })),
      toDOM: node => [`h${node.attrs.level}`, 0],
    };
  }
  if (has('quote')) {
    nodes.blockquote = { content: 'block+', group: 'block', defining: true, parseDOM: [{ tag: 'blockquote' }], toDOM: () => ['blockquote', 0] };
  }
  if (has('codeBlock')) {
    nodes.code_block = {
      attrs: { language: { default: null } },
      content: 'text*',
      marks: '',
      group: 'block',
      code: true,
      defining: true,
      parseDOM: [{
        tag: 'pre',
        preserveWhitespace: 'full',
        getAttrs: dom => {
          const code = dom.querySelector('code');
          const cls = (code?.getAttribute('class') || '').match(/(?:^|\s)language-(\S+)/);
          return { language: languageOf(code?.getAttribute('data-language') || cls?.[1]) };
        },
      }],
      toDOM: node => ['div', { class: 'wk-code' }, ['pre', ['code', node.attrs.language ? { 'data-language': node.attrs.language } : {}, 0]]],
    };
  }
  if (has('rule')) {
    nodes.horizontal_rule = { group: 'block', parseDOM: [{ tag: 'hr' }], toDOM: () => ['hr'] };
  }
  if (has('bulletList')) {
    nodes.bullet_list = {
      attrs: { tight: { default: true } },
      content: 'list_item+',
      group: 'block',
      parseDOM: [{ tag: 'ul', getAttrs: dom => ({ tight: !dom.querySelector(':scope > li > p') }) }],
      toDOM: node => ['ul', node.attrs.tight ? { 'data-tight': '' } : {}, 0],
    };
  }
  if (has('orderedList')) {
    nodes.ordered_list = {
      attrs: { order: { default: 1 }, tight: { default: true } },
      content: 'list_item+',
      group: 'block',
      parseDOM: [{
        tag: 'ol',
        getAttrs: dom => {
          const start = Number.parseInt(dom.getAttribute('start') ?? '1', 10);
          return { order: Number.isInteger(start) && start >= 0 && start <= 999999999 ? start : 1, tight: !dom.querySelector(':scope > li > p') };
        },
      }],
      toDOM: node => ['ol', { ...(node.attrs.order === 1 ? {} : { start: String(node.attrs.order) }), ...(node.attrs.tight ? { 'data-tight': '' } : {}) }, 0],
    };
  }
  if (lists) {
    nodes.list_item = { content: 'paragraph block*', defining: true, parseDOM: [{ tag: 'li' }], toDOM: () => ['li', 0] };
  }
  nodes.text = { group: 'inline' };
  nodes.hard_break = { inline: true, group: 'inline', selectable: false, parseDOM: [{ tag: 'br' }], toDOM: () => ['br'] };

  // Marks, outermost first: a link holds bold, bold holds italic, and code
  // is innermost (Markdown cannot format inside a code span).
  const marks = {};
  if (has('link')) {
    marks.link = {
      attrs: { href: {} },
      inclusive: false,
      parseDOM: [{ tag: 'a[href]', getAttrs: dom => { const href = linkTarget(dom.getAttribute('href')); return href ? { href } : false; } }],
      toDOM: mark => ['a', { class: 'wk-link', href: safeUrl(mark.attrs.href) || '', rel: 'noopener noreferrer' }, 0],
    };
  }
  if (has('bold')) {
    marks.strong = {
      parseDOM: [
        { tag: 'strong' },
        // Google Docs wraps text in <b style="font-weight: normal">.
        { tag: 'b', getAttrs: dom => !isNotBold(dom) && null },
        { tag: 'span[data-wk-style]', getAttrs: dom => isBold(dom) && null, consuming: false },
      ],
      toDOM: () => ['strong', 0],
    };
  }
  if (has('italic')) {
    marks.em = { parseDOM: [{ tag: 'em' }, { tag: 'i' }, { tag: 'span[data-wk-style]', getAttrs: dom => isItalic(dom) && null, consuming: false }], toDOM: () => ['em', 0] };
  }
  if (has('code')) {
    marks.code = { parseDOM: [{ tag: 'code' }], toDOM: () => ['code', 0] };
  }
  return new Schema({ nodes, marks });
}
