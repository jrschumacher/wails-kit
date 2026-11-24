// Link safety for the editors: the URLs a link may carry (http, https and
// mailto, as the browser's URL parser reads them) and HTML character
// reference decoding. Shared by the Markdown renderer and the rich-text
// editor; no DOM, so node tests it.

const ENTITIES = {
  amp: '&', lt: '<', gt: '>', quot: '"', apos: '\'', nbsp: ' ', copy: '©', reg: '®', trade: '™',
  hellip: '…', mdash: '—', ndash: '–', lsquo: '‘', rsquo: '’', ldquo: '“', rdquo: '”', laquo: '«', raquo: '»',
  times: '×', divide: '÷', middot: '·', bull: '•', deg: '°', plusmn: '±', para: '¶', sect: '§', euro: '€',
  pound: '£', yen: '¥', cent: '¢', larr: '←', rarr: '→', uarr: '↑', darr: '↓', harr: '↔', le: '≤', ge: '≥', ne: '≠',
};

/**
 * Decodes HTML character references once: numeric ones and a set of common
 * named ones (marked leaves them encoded); an unknown name stays as written.
 */
export function decodeEntities(text) {
  return String(text).replace(/&(#x[0-9a-f]{1,6}|#[0-9]{1,7}|[a-z][a-z0-9]{1,31});/gi, (whole, ref) => {
    if (ref[0] !== '#') return Object.hasOwn(ENTITIES, ref) ? ENTITIES[ref] : whole;
    const code = ref[1] === 'x' || ref[1] === 'X' ? parseInt(ref.slice(2), 16) : parseInt(ref.slice(1), 10);
    return code > 0 && code <= 0x10ffff && !(code >= 0xd800 && code <= 0xdfff) ? String.fromCodePoint(code) : '�';
  });
}

const LINK_PROTOCOLS = new Set(['http:', 'https:', 'mailto:']);

/**
 * The URL a link may carry, normalised, or null: an absolute http, https or
 * mailto URL. Character references are decoded first and the browser's own
 * URL parser decides the scheme, so "&#106;avascript:" and "java\tscript:"
 * are refused like "javascript:".
 */
export function safeUrl(href) {
  if (typeof href !== 'string') return null;
  let url;
  try { url = new URL(decodeEntities(href).trim()); } catch { return null; }
  return LINK_PROTOCOLS.has(url.protocol) ? url.href : null;
}
