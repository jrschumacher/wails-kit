// The Markdown limits scan, shared by the Markdown renderer and editor and
// the rich-text editor: text past a limit is never handed to a recursive
// parser (marked's lexer, CodeMirror's Markdown parser). No DOM, so node
// tests it (test/markdown.test.mjs).
//
// Limits. marked's lexer (and CodeMirror's Markdown parser and highlighter)
// recurse for each nested quote, list, emphasis run and bracket, so hostile
// text a few kilobytes long can overflow the stack, and a deeply nested list
// takes time quadratic in its depth. Text past a limit, or that fails to
// parse, renders as its source in one plain paragraph.
/** Longest text rendered as Markdown, in characters (UTF-16 code units). */
export const MAX_MARKDOWN_LENGTH = 100000;
/** Deepest nesting of quotes and lists rendered as Markdown. */
export const MAX_MARKDOWN_DEPTH = 32;
/** Most emphasis and link delimiters (* _ ~ [ ]) in one run of inline text (see markdownLimits) rendered as Markdown. */
export const MAX_MARKDOWN_DELIMITERS = 2000;
/**
 * Most delimiters between two blank lines or headings, whatever the runs in
 * between: a backstop that keeps the parser's time bounded (about 0.7 s in
 * the worst case) even if a run reset is wrong.
 */
export const MAX_MARKDOWN_BLOCK_DELIMITERS = 8000;

const container = /^[ \t]*(?:>|[-*+](?=[ \t]|$)|\d{1,9}[.)](?=[ \t]|$))[ \t]?/;
// One quote marker, as CommonMark allows it (up to three spaces before).
const quoteMarker = /^ {0,3}>[ \t]?/;
// A fence opening: three or more backticks (whose info string has none) or tildes.
const opening = /^( {0,3})(`{3,}|~{3,})(.*)$/;
const closing = /^[ \t]*(`{3,}|~{3,})[ \t]*$/;
const heading = /^ {0,3}#{1,6}(?:[ \t]|$)/;
const tableDelimiterRow = /^[ \t]*\|?[ \t]*:?-+:?[ \t]*(?:\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$/;
const delimiter = /[*_~[\]]/g;
// A table row's cells, as GFM counts them: split on pipes that are not
// escaped, without the optional pipes at either end.
function cellCount(row) {
  let text = row.trim();
  if (text.startsWith('|')) text = text.slice(1);
  if (text.endsWith('|') && !text.endsWith('\\|')) text = text.slice(0, -1);
  return text.split(/(?<!\\)\|/).length;
}
const columns = ws => ws.replace(/\t/g, '    ').length;
const leading = line => columns(line.match(/^[ \t]*/)[0]);

/**
 * Scans the text for the limits above: { depth, delimiters, blocks }, the deepest
 * nesting of quotes and lists on a line (the markers it starts with, plus
 * its list indentation at two columns a level) and the most delimiters in
 * one run of text emphasis could span, both outside fenced code. A run ends
 * where Markdown ends inline content: a blank line, a heading, a new list
 * item, a table row; blocks is the most between blank lines and headings,
 * whatever the runs. It errs on the high side: where the parser might read a
 * line either way (a fence that its quote or list item ends, a list marker
 * that may only continue a paragraph), the line is counted.
 */
export function markdownLimits(text) {
  let depth = 0, delimiters = 0, run = 0, block = 0, blocks = 0, fence = null, header = null;
  // The content column of the last list item line, whether the last
  // non-blank line started an item, and whether a table is open.
  let itemColumn = 0, afterItem = false, table = false;
  for (const line of String(text ?? '').split(/\r\n?|\n/)) {
    if (fence) {
      // Fenced code ends with its container: a line that does not repeat the
      // fence's quote markers, or (non-blank) is less indented than the
      // fence, is read as text again.
      let inner = line, quotes = 0, q;
      while (quotes < fence.quotes && (q = inner.match(quoteMarker))) { inner = inner.slice(q[0].length); quotes++; }
      if (quotes === fence.quotes) {
        const c = inner.match(closing);
        if (c && c[1][0] === fence.char && c[1].length >= fence.length) { fence = null; continue; }
        if (!inner.trim() || leading(inner) >= fence.column) continue;
      }
      fence = null;
    }
    if (!line.trim()) { run = 0; block = 0; table = false; header = null; continue; }
    let rest = line, level = 0, quotes = 0, list = null, m;
    while ((m = rest.match(container))) {
      const marker = m[0].trim();
      if (marker === '>' && !list && level === quotes) quotes++;
      else if (marker !== '>' && !list && level === quotes) list = { marker, column: columns(line.slice(0, line.length - rest.length)) + leading(rest) };
      level++;
      rest = rest.slice(m[0].length);
      if (level > MAX_MARKDOWN_DEPTH) break;
    }
    if (level) level += Math.floor(leading(line) / 2);
    if (level > depth) depth = level;
    const f = rest.match(opening);
    if (f && !(f[2][0] === '`' && f[3].includes('`'))) {
      // The fence's column, counted after its quote markers.
      let afterQuotes = line;
      for (let i = 0; i < quotes; i++) afterQuotes = afterQuotes.slice(afterQuotes.match(quoteMarker)?.[0].length ?? 0);
      fence = { char: f[2][0], length: f[2].length, quotes, column: columns(afterQuotes.slice(0, afterQuotes.length - rest.length)) + f[1].length };
      run = 0;
      block = 0;
      header = null;
      continue;
    }
    // A list item ends the run before it. Only a marker that must start an
    // item counts: not in a quote, with content, no deeper than three
    // columns past the last item's content (deeper, it may continue a
    // paragraph), and a bullet, a 1., or a number right after another item
    // (another number cannot interrupt a paragraph).
    const item = !!list && quotes === 0 && rest.trim() !== '' && list.column <= itemColumn + 3
      && (/^[-*+]$/.test(list.marker) || /^0*1[.)]$/.test(list.marker) || afterItem);
    if (item) { run = 0; itemColumn = columns(line.slice(0, line.length - rest.length)); }
    const isHeading = heading.test(rest);
    // A table row: inline content stays in its cell.
    const row = table && rest.includes('|');
    if (!row) table = false;
    if (isHeading || row) run = 0;
    if (isHeading) block = 0;
    const count = (rest.match(delimiter) || []).length;
    run += count;
    block += count;
    if (run > delimiters) delimiters = run;
    if (block > blocks) blocks = block;
    if (isHeading || row) run = 0;
    if (isHeading) block = 0;
    // A delimiter row makes a table only under a header row with as many
    // cells (GFM); otherwise it is text and resets nothing.
    if (!table && header !== null && rest.includes('|') && tableDelimiterRow.test(rest) && cellCount(rest) === cellCount(header)) { table = true; run = 0; }
    header = !table && rest.includes('|') ? rest : null;
    afterItem = item;
    if (depth > MAX_MARKDOWN_DEPTH && delimiters > MAX_MARKDOWN_DELIMITERS) break;
  }
  return { depth, delimiters, blocks };
}

/** Whether markdownLimits' counts are past a limit (the length aside). */
export function exceedsMarkdownLimits({ depth, delimiters, blocks }) {
  return depth > MAX_MARKDOWN_DEPTH || delimiters > MAX_MARKDOWN_DELIMITERS || blocks > MAX_MARKDOWN_BLOCK_DELIMITERS;
}

/** markdownLimits(text).depth. */
export function nestingDepth(text) {
  return markdownLimits(text).depth;
}

/**
 * Whether the text is past a limit (length, depth or delimiters), so it
 * renders as its source. The editor uses it to drop Markdown highlighting.
 */
export function pastMarkdownLimits(text) {
  text = String(text ?? '');
  if (text.length > MAX_MARKDOWN_LENGTH) return true;
  return exceedsMarkdownLimits(markdownLimits(text));
}
