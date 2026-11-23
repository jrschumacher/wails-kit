// The languages the code editor bundles, by name, alias and file extension.
// Another language comes in as a CodeMirror stream parser (a legacy mode,
// such as those in @codemirror/legacy-modes): it is plain data with no
// CodeMirror imports, so it works with this bundle's copy of CodeMirror.
import { LanguageSupport, StreamLanguage } from '@codemirror/language';
import { jsxLanguage, tsxLanguage } from '@codemirror/lang-javascript';
import { goLanguage } from '@codemirror/lang-go';
import { jsonLanguage } from '@codemirror/lang-json';
import { yamlLanguage } from '@codemirror/lang-yaml';
import { markdown } from '@codemirror/lang-markdown';
import { cssLanguage } from '@codemirror/lang-css';
import { htmlLanguage } from '@codemirror/lang-html';
import { shell } from '@codemirror/legacy-modes/mode/shell';

// The bare languages (parser, highlighting, indentation, folding), without
// the packages' autocompletion sources and snippets, which the editor does
// not offer. JavaScript and TypeScript parse JSX.
const bare = language => () => new LanguageSupport(language);
const make = {
  javascript: bare(jsxLanguage),
  typescript: bare(tsxLanguage),
  go: bare(goLanguage),
  json: bare(jsonLanguage),
  yaml: bare(yamlLanguage),
  // Fenced code blocks take the bundled languages too.
  markdown: () => markdown({ codeLanguages: info => resolve(info)?.language ?? null, completeHTMLTags: false }),
  shell: () => new LanguageSupport(StreamLanguage.define(shell)),
  css: bare(cssLanguage),
  html: bare(htmlLanguage),
};

/** The names of the bundled languages. */
export const languages = Object.freeze(Object.keys(make));

const aliases = {
  js: 'javascript', jsx: 'javascript', mjs: 'javascript', cjs: 'javascript', node: 'javascript',
  ts: 'typescript', tsx: 'typescript', mts: 'typescript', cts: 'typescript',
  golang: 'go',
  jsonc: 'json', json5: 'json',
  yml: 'yaml',
  md: 'markdown', mdx: 'markdown',
  sh: 'shell', bash: 'shell', zsh: 'shell', console: 'shell',
  scss: 'css',
  htm: 'html', xhtml: 'html',
};

const cache = new Map();
const parsers = new WeakMap();

/** The bundled language name for a name or alias (any case), or null. */
export function languageName(name) {
  if (typeof name !== 'string') return null;
  const key = name.trim().toLowerCase();
  return make[key] ? key : aliases[key] ?? null;
}

/** A bundled language's LanguageSupport by name or alias, or null. */
export function resolve(name) {
  const id = languageName(name);
  if (!id) return null;
  if (!cache.has(id)) cache.set(id, make[id]());
  return cache.get(id);
}

/** The bundled language name for a file path, by its extension, or null. */
export function languageForPath(path) {
  if (typeof path !== 'string') return null;
  const base = path.split(/[\\/]/).pop().toLowerCase();
  if (/^\.(bashrc|zshrc|profile|bash_profile|envrc)$/.test(base)) return 'shell';
  const dot = base.lastIndexOf('.');
  return dot > 0 ? languageName(base.slice(dot + 1)) : null;
}

/**
 * The language extension for an option value: a bundled name, a CodeMirror
 * stream parser ({ token(stream, state) … }), or nothing (plain text).
 * An unknown name throws, so a typo does not pass silently.
 */
export function languageExtension(language) {
  if (language == null || language === '' || language === 'text' || language === 'plain') return [];
  if (typeof language === 'string') {
    const support = resolve(language);
    if (!support) throw new TypeError(`wkCodeEditor: unknown language "${language}"; bundled: ${languages.join(', ')}`);
    return support;
  }
  if (typeof language === 'object' && typeof language.token === 'function') {
    if (!parsers.has(language)) parsers.set(language, new LanguageSupport(StreamLanguage.define(language)));
    return parsers.get(language);
  }
  throw new TypeError('wkCodeEditor: language is a bundled language name or a CodeMirror stream parser');
}
