// The editors' syntax colours for read-only code: highlighter() (the diff
// view's highlight hook, one line at a time) and highlightBlock() (a whole
// block, for the Markdown preview's code blocks).
import { highlightCode } from '@lezer/highlight';
import { StyleModule } from 'style-mod';
import { highlightStyle } from './theme.js';
import { languageForPath, resolve, languages } from './languages.js';

let mounted = false;

// text in a LanguageSupport (or none: plain text) as a DocumentFragment of
// spans carrying the syntax classes.
function paint(text, support) {
  const fragment = document.createDocumentFragment();
  if (!support || !text) { if (text) fragment.append(text); return fragment; }
  if (!mounted) { StyleModule.mount(document, highlightStyle.module); mounted = true; }
  highlightCode(text, support.language.parser.parse(text), highlightStyle, (part, classes) => {
    if (!classes) { fragment.append(part); return; }
    const span = document.createElement('span');
    span.className = classes;
    span.textContent = part;
    fragment.append(span);
  }, () => fragment.append('\n'));
  return fragment;
}

/**
 * The diff view's highlight hook: highlighter(language) returns
 * highlight(file, line) → DocumentFragment, colouring one line's text with
 * the editor's syntax colours. With no language it picks one from the file's
 * path (file.path, else file.oldPath); a line in no known language comes back
 * as plain text. `line` is a diff line ({ text }) or a string. Each line is
 * highlighted on its own, so a construct that spans lines (a block comment)
 * colours only the line it starts on.
 */
export function highlighter(language) {
  const fixed = language == null || language === '' ? null : resolve(language);
  if (language != null && language !== '' && !fixed) throw new TypeError(`highlighter: unknown language "${language}"; bundled: ${languages.join(', ')}`);
  return function highlight(file, line) {
    const text = typeof line === 'string' ? line : String(line?.text ?? '');
    return paint(text, fixed ?? resolve(languageForPath(file?.path || file?.oldPath)));
  };
}

/**
 * A whole block of code in a bundled language (a name or alias) as a
 * DocumentFragment of spans in the syntax colours; plain text for no language
 * or one not bundled. Unlike highlighter(), the block is parsed as one, so a
 * construct spanning lines is coloured throughout.
 */
export function highlightBlock(text, language) {
  return paint(String(text ?? ''), resolve(language));
}
