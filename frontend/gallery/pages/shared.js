// Glyphs and small builders the pages share. Glyphs are inline SVG in
// currentColor, as an app draws them.
const icon = path => `<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">${path}</svg>`;
export const icons = {
  now: icon('<path d="M2.5 11a5.5 5.5 0 0 1 11 0"/><path d="M8 11l2.5-3"/>'),
  history: icon('<path d="M3 13V8M8 13V3M13 13V6"/>'),
  items: icon('<rect x="2.5" y="3" width="11" height="4" rx="1"/><rect x="2.5" y="9" width="11" height="4" rx="1"/>'),
  book: icon('<path d="M4 2.5h8v11l-4-2.5-4 2.5z"/>'),
  panel: icon('<rect x="2" y="3" width="12" height="10" rx="1.5"/><path d="M6 3v10"/>'),
  lock: icon('<rect x="3" y="7" width="10" height="7" rx="1.3"/><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2"/>'),
  eye: icon('<path d="M1.5 8S4 3.5 8 3.5 14.5 8 14.5 8 12 12.5 8 12.5 1.5 8 1.5 8z"/><circle cx="8" cy="8" r="2"/>'),
  copy: icon('<rect x="5.5" y="5.5" width="8" height="8" rx="1.6"/><path d="M10.5 3.5v-.2a1.3 1.3 0 0 0-1.3-1.3H3.8a1.3 1.3 0 0 0-1.3 1.3v5.4a1.3 1.3 0 0 0 1.3 1.3h.2"/>'),
  warn: icon('<circle cx="8" cy="8" r="6.3"/><path d="M8 4.8v4M8 11v.3"/>'),
};
const statGlyph = path => `<svg viewBox="0 0 18 18" aria-hidden="true"><g fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">${path}</g></svg>`;
export const statGlyphs = {
  grid: statGlyph('<rect x="3" y="3" width="5" height="5" rx="1" fill="currentColor"/><rect x="10" y="3" width="5" height="5" rx="1"/><rect x="3" y="10" width="5" height="5" rx="1"/><rect x="10" y="10" width="5" height="5" rx="1"/>'),
  ok: statGlyph('<path d="M9 2.5 14.5 4.5V9c0 3.2-2.3 5.4-5.5 6.5C5.8 14.4 3.5 12.2 3.5 9V4.5z"/><path d="M6.6 9l1.8 1.8 3.2-3.4"/>'),
  time: statGlyph('<circle cx="9" cy="9" r="6.5"/><path d="M9 5.5V9l2.5 1.5"/>'),
};
const glyph = path => `<svg viewBox="0 0 22 22" aria-hidden="true"><g fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">${path}</g></svg>`;
export const glyphs = {
  general: glyph('<circle cx="11" cy="11" r="3"/><path d="M11 2.5v2.5M11 17v2.5M2.5 11H5M17 11h2.5M5 5l1.8 1.8M15.2 15.2 17 17M5 17l1.8-1.8M15.2 6.8 17 5"/>'),
  sharing: glyph('<rect x="4" y="4.5" width="14" height="10" rx="1.3"/><path d="M2 17.5h18"/>'),
  about: glyph('<circle cx="11" cy="11" r="8.5"/><path d="M11 10v5.5M11 6.8v.2"/>'),
};
export const stepper = (id, label, value, { more = true, fewer = true, text = false, invalid = false } = {}) => `<span class="wk-stepper"><input${id ? ` id="${id}"` : ''} class="wk-field" ${text ? '' : 'type="number" '}inputmode="numeric" min="1" max="8" value="${value}" aria-label="${label}"${invalid ? ' aria-invalid="true"' : ''}><span class="wk-stepper-arrows"><button type="button" aria-label="More"${more ? '' : ' disabled'}></button><button type="button" aria-label="Fewer"${fewer ? '' : ' disabled'}></button></span></span>`;
// A "Try it" button.
export const tryButton = (id, label = 'Try it') => `<button class="wk-button" type="button" id="${id}" data-gallery-try>${label}</button>`;
