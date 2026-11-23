// The CodeMirror theme and syntax highlight style built from theme-spec.js.
// Shared by the editors; each entry bundles its own copy.
import { EditorView } from '@codemirror/view';
import { HighlightStyle } from '@codemirror/language';
import { tags } from '@lezer/highlight';
import { editorTheme, syntax } from './theme-spec.js';

// "function(variableName)" → tags.function(tags.variableName).
function tag(name) {
  const m = name.match(/^(\w+)\((.+)\)$/);
  const t = m ? tags[m[1]]?.(tag(m[2])) : tags[name];
  if (!t) throw new Error(`theme-spec.js: unknown highlight tag ${name}`);
  return t;
}

export const theme = EditorView.theme(editorTheme);

export const highlightStyle = HighlightStyle.define(syntax.map(([names, style]) => ({ tag: names.map(tag), ...style })));
