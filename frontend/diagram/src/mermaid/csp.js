// Hooks that scripts/build.mjs patches into Mermaid and d3 so they draw
// under a Content-Security-Policy with no 'unsafe-inline' style-src (the
// kit's pages use default-src 'self'). Mermaid styles a diagram three ways,
// each of which such a policy blocks and reports as a violation:
//   - a <style> element in the SVG: its rules go to a constructable
//     stylesheet adopted by the document instead (adoptRules);
//   - style attributes set while drawing: set through the CSSOM instead,
//     which a CSP does not block (setStyle);
//   - style attributes in the markup it serializes and sanitizes with
//     DOMPurify, whose parser document shares the page's CSP: they travel
//     as data-wk-style and go back through the CSSOM once the SVG is in the
//     page (stashStyles here, restoreStyles in index.js).
// Every rule, declaration and attribute that names a url() other than a
// fragment (#id) is dropped, so a diagram's source cannot make the page
// fetch anything. Shapes that load a picture (image and icon shapes) are
// refused before they draw (refuse).

const sheets = new Map(); // diagram id -> its CSSStyleSheet

// A CSS value that loads something. The CSSOM has resolved escapes
// (u\72l) by the time a declaration's value is read.
const external = value => /url\(\s*(?!['"]?#)/i.test(value) || /@import|image-set\(|image\(/i.test(value);

/**
 * An attribute value that could load something: a url() that is not a
 * fragment, in a presentation attribute (fill, filter, mask, marker-end,
 * cursor, …), which is parsed as CSS, so a backslash escape counts too.
 */
export const PRESENTATION = name => !/^(?:(?:xlink:)?href|id|class|d|points|transform|viewBox|role|aria-.*|data-.*)$/i.test(String(name));
export const loadsInAttribute = value => /url\(\s*(?!['"]?#)|image-set\(|\\/i.test(String(value));

/** Stops a shape that would load a picture: Mermaid rejects the render with this error. */
export function refuse(what) {
  const err = new Error(`${what} in diagrams are not supported.`);
  err.wkKind = 'unsupported';
  throw err;
}

/** Removes every declaration in a CSSStyleDeclaration that loads something. */
export function scrubDeclaration(style) {
  for (const prop of [...style]) if (external(style.getPropertyValue(prop))) style.removeProperty(prop);
}

// Drops @font-face rules (a font loads from a URL) and every declaration
// that loads something; `owner` is a stylesheet or a grouping rule.
function scrubRules(owner) {
  for (let i = owner.cssRules.length - 1; i >= 0; i--) {
    const rule = owner.cssRules[i];
    if (rule instanceof CSSFontFaceRule || rule instanceof CSSImportRule) { owner.deleteRule(i); continue; }
    if (rule.style) scrubDeclaration(rule.style);
    if (rule.cssRules) scrubRules(rule);
  }
}

/** Mermaid's styles for the diagram, adopted by the document; keyed by the SVG's id. */
export function adoptRules(svg, rules) {
  const id = svg?.id;
  if (!id) throw new Error('@wails-kit/diagram: Mermaid\'s SVG has no id; scripts/build.mjs PATCHES needs review');
  const doc = svg.ownerDocument;
  if (!('adoptedStyleSheets' in doc) || typeof CSSStyleSheet !== 'function') {
    // No constructable stylesheets (Safari before 16.4): a <style> element,
    // which a strict CSP blocks. index.js warns once.
    const style = doc.createElement('style');
    style.textContent = rules;
    svg.insertBefore(style, svg.firstChild);
    return;
  }
  const sheet = new CSSStyleSheet();
  sheet.replaceSync(rules); // replaceSync ignores @import
  scrubRules(sheet);
  sheets.get(id)?.remove();
  doc.adoptedStyleSheets = [...doc.adoptedStyleSheets, sheet];
  sheets.set(id, {
    sheet,
    remove() { doc.adoptedStyleSheets = doc.adoptedStyleSheets.filter(s => s !== sheet); },
    restore() { if (!doc.adoptedStyleSheets.includes(sheet)) doc.adoptedStyleSheets = [...doc.adoptedStyleSheets, sheet]; },
  });
}

/** Takes the stylesheet adoptRules made for `id` (the caller removes it later). */
export function takeSheet(id) {
  const entry = sheets.get(id);
  sheets.delete(id);
  return entry || null;
}

/** el.setAttribute('style', value), through the CSSOM. */
// A declaration list without the declarations that could load something,
// whole: `mask: url(…)` goes, not just the mask-image it expands to. Split
// at semicolons outside brackets and quotes.
export function dropLoading(text) {
  const out = [];
  let depth = 0, quote = '', start = 0;
  for (let i = 0; i <= text.length; i++) {
    const c = text[i];
    if (quote) { if (c === '\\') i++; else if (c === quote) quote = ''; continue; }
    if (c === '"' || c === "'") quote = c;
    else if (c === '(') depth++;
    else if (c === ')') depth = Math.max(0, depth - 1);
    else if ((c === ';' && depth === 0) || i === text.length) {
      const decl = text.slice(start, i).trim();
      if (decl && !loadsInAttribute(decl)) out.push(decl);
      start = i + 1;
    }
  }
  return out.join('; ');
}

export function setStyle(el, value) {
  if (value == null) { el.removeAttribute('style'); return; }
  if (!el.style) { el.setAttribute('style', dropLoading(String(value))); return; }
  el.style.cssText = dropLoading(String(value));
  scrubDeclaration(el.style);
}

/**
 * The setAttribute d3 and Mermaid call: "style" goes through the CSSOM, and
 * a value that loads something (fill="url(//host/x.svg#a)") is not set.
 */
export function setAttr(el, name, value) {
  if (String(name).toLowerCase() === 'style') setStyle(el, value);
  else if (value != null && PRESENTATION(name) && loadsInAttribute(value)) el.removeAttribute(name);
  else el.setAttribute(name, value);
}

/** Before Mermaid serializes the SVG: style attributes become data-wk-style, <style> elements go. */
export function stashStyles(root) {
  if (!root) return;
  for (const el of root.querySelectorAll('[style]')) {
    el.setAttribute('data-wk-style', el.getAttribute('style'));
    el.removeAttribute('style');
  }
  for (const el of root.querySelectorAll('style')) el.remove();
}
