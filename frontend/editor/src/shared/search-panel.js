// CodeMirror's find-and-replace panel, drawn with the kit's field, button and
// checkbox classes (@wails-kit/ui's stylesheet styles them). Mod-F opens it,
// Escape closes it and returns focus to the text.
import { runScopeHandlers } from '@codemirror/view';
import {
  SearchQuery, getSearchQuery, setSearchQuery, findNext, findPrevious, selectMatches,
  replaceNext, replaceAll, closeSearchPanel,
} from '@codemirror/search';

const SVG = 'http://www.w3.org/2000/svg';

function el(tag, attrs = {}, children = []) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k.startsWith('on')) node.addEventListener(k.slice(2), v);
    else if (v === true) node.setAttribute(k, '');
    else if (v !== false && v != null) node.setAttribute(k, v);
  }
  node.append(...children);
  return node;
}

function closeIcon() {
  const svg = document.createElementNS(SVG, 'svg');
  svg.setAttribute('viewBox', '0 0 16 16');
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '1.5');
  svg.setAttribute('aria-hidden', 'true');
  const path = document.createElementNS(SVG, 'path');
  path.setAttribute('d', 'M4 4l8 8M12 4l-8 8');
  svg.append(path);
  return svg;
}

class KitSearchPanel {
  constructor(view) {
    this.view = view;
    const query = this.query = getSearchQuery(view.state);
    const commit = () => this.commit();
    const phrase = text => view.state.phrase(text);
    const field = (name, label, value, main) => el('input', {
      class: 'wk-field', type: 'text', name, value, placeholder: phrase(label), 'aria-label': phrase(label),
      autocomplete: 'off', spellcheck: 'false', 'main-field': main ? 'true' : null, onchange: commit, onkeyup: commit,
    });
    const check = (name, label, checked) => {
      const input = el('input', { class: 'wk-checkbox', type: 'checkbox', name, onchange: commit });
      input.checked = checked;
      return [input, el('label', { class: 'wk-check' }, [input, phrase(label)])];
    };
    const button = (label, run, name) => el('button', { class: 'wk-button wk-button--small', type: 'button', 'aria-label': name ? phrase(name) : null, onclick: () => run(view) }, [phrase(label)]);
    this.searchField = field('search', 'Find', query.search, true);
    this.replaceField = field('replace', 'Replace', query.replace, false);
    let caseLabel, reLabel, wordLabel;
    [this.caseField, caseLabel] = check('case', 'Match case', query.caseSensitive);
    [this.reField, reLabel] = check('re', 'Regexp', query.regexp);
    [this.wordField, wordLabel] = check('word', 'By word', query.wholeWord);
    const close = el('button', {
      class: 'wk-button wk-button--small wk-button--quiet wk-button--icon wk-code-search-close', type: 'button',
      'aria-label': phrase('Close find'), onclick: () => closeSearchPanel(view),
    }, [closeIcon()]);
    const find = el('div', { class: 'wk-code-search-row' }, [
      this.searchField, button('Next', findNext), button('Previous', findPrevious), button('All', selectMatches, 'Select all matches'),
      caseLabel, reLabel, wordLabel, close,
    ]);
    const rows = [find];
    if (!view.state.readOnly) {
      rows.push(el('div', { class: 'wk-code-search-row' }, [this.replaceField, button('Replace', replaceNext), button('Replace all', replaceAll)]));
    }
    this.dom = el('div', { class: 'wk-code-search', role: 'search', 'aria-label': phrase('Find'), onkeydown: e => this.keydown(e) }, rows);
  }

  commit() {
    const query = new SearchQuery({
      search: this.searchField.value, replace: this.replaceField.value,
      caseSensitive: this.caseField.checked, regexp: this.reField.checked, wholeWord: this.wordField.checked,
    });
    if (!query.eq(this.query)) {
      this.query = query;
      this.view.dispatch({ effects: setSearchQuery.of(query) });
    }
  }

  keydown(e) {
    if (runScopeHandlers(this.view, e, 'search-panel')) e.preventDefault();
    else if (e.key === 'Enter' && e.target === this.searchField) { e.preventDefault(); (e.shiftKey ? findPrevious : findNext)(this.view); }
    else if (e.key === 'Enter' && e.target === this.replaceField) { e.preventDefault(); replaceNext(this.view); }
  }

  update(update) {
    for (const tr of update.transactions) for (const effect of tr.effects) {
      if (effect.is(setSearchQuery) && !effect.value.eq(this.query)) this.setQuery(effect.value);
    }
  }

  setQuery(query) {
    this.query = query;
    this.searchField.value = query.search;
    this.replaceField.value = query.replace;
    this.caseField.checked = query.caseSensitive;
    this.reField.checked = query.regexp;
    this.wordField.checked = query.wholeWord;
  }

  mount() { this.searchField.select(); }

  get top() { return true; }
}

export const createSearchPanel = view => new KitSearchPanel(view);
