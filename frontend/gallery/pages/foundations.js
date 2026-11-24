// Foundations: the type scale, the tokens, the status dot, motion and timings.
export default [
  {
    id: 'type', title: 'Type scale', group: 'foundations',
    intro: 'Inter, bundled with the kit; the classes set size, weight and colour. Numbers that change in place use tabular numerals.',
    examples: [{
      title: 'Every style',
      html: `<div class="g-stack">
  <p class="wk-title">Title, 24 px</p>
  <p class="wk-title-2">Title 2, 20 px</p>
  <p class="wk-heading">Heading, 15 px semibold</p>
  <p class="wk-subheading">Subheading, 13 px semibold</p>
  <p class="wk-body">Body, 13 px. The quick brown fox jumps over the lazy dog.</p>
  <p class="wk-caption">Caption, 12 px secondary</p>
  <p class="wk-small">Small, 11 px secondary</p>
  <p><span class="wk-figure">129</span> <span class="wk-secondary">figure, tabular numerals:</span> <span class="wk-num">11:02 0:41 3:18</span></p>
  <p class="wk-mono">wk-mono: join:v1:AAAA-BBBB</p>
  <p><code>code: system monospace</code></p>
  <p>Visible words <span class="wk-visually-hidden">and words only a screen reader reads</span></p>
</div>`,
    }],
  },
  {
    id: 'tokens', title: 'Tokens', group: 'foundations',
    intro: 'Every <code>--wk-*</code> custom property the page declares, with its value now: they follow the appearance, the theme and increased contrast.',
    examples: [{
      title: 'Colours, shadows and the other values',
      html: '<div class="g-swatches" id="g-swatches"></div><dl class="g-values" id="g-values"></dl>',
      after(el) {
        // Reads every --wk-* custom property the page's :root rules declare.
        const draw = () => {
          const names = new Set();
          const walk = rules => {
            for (const rule of rules) {
              if (rule.cssRules) walk(rule.cssRules);
              if (rule.styleSheet) walk(rule.styleSheet.cssRules);
              if (/^:root\b/.test(rule.selectorText || '')) for (const p of rule.style) if (p.startsWith('--wk-')) names.add(p);
            }
          };
          for (const sheet of document.styleSheets) walk(sheet.cssRules);
          const root = getComputedStyle(document.documentElement);
          const swatches = el.querySelector('.g-swatches');
          const values = el.querySelector('.g-values');
          swatches.replaceChildren();
          values.replaceChildren();
          for (const name of [...names].sort()) {
            const value = root.getPropertyValue(name).trim();
            if (name.startsWith('--wk-color-') || name.startsWith('--wk-shadow-')) {
              const card = document.createElement('figure');
              card.className = 'g-swatch';
              const chip = document.createElement('span');
              chip.className = 'g-chip';
              if (name.startsWith('--wk-color-')) chip.style.background = `var(${name})`;
              else chip.style.boxShadow = `var(${name})`;
              const cap = document.createElement('figcaption');
              const code = document.createElement('code');
              code.textContent = name.slice(5);
              const small = document.createElement('span');
              small.className = 'wk-small';
              small.textContent = value;
              cap.append(code, small);
              card.append(chip, cap);
              swatches.append(card);
            } else {
              const dt = document.createElement('dt');
              const dd = document.createElement('dd');
              dt.textContent = name.slice(5);
              dd.textContent = value;
              values.append(dt, dd);
            }
          }
        };
        draw();
        const watch = new MutationObserver(draw);
        watch.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
        document.addEventListener('gallery-theme', draw);
        return () => { watch.disconnect(); document.removeEventListener('gallery-theme', draw); };
      },
    }],
  },
  {
    id: 'dot', title: 'Status dot', group: 'foundations', card: 'StatusDot',
    intro: 'Six shapes by three tones, at the small (8 px), default and a 14 px size. There is no warning tone, and no state names: the app maps its states onto shapes and tones. A filled working dot at the default size and above has the halo (running).',
    examples: [{
      title: 'Shapes and tones',
      html: `<table class="wk-table g-dots">
  <thead><tr><th>Shape</th>${['neutral', 'working', 'attention'].map(t => `<th>${t}</th>`).join('')}</tr></thead>
  <tbody>${['filled', 'ringed', 'half', 'hollow', 'dashed', 'bar'].map(s => `<tr><th scope="row">${s}</th>${['neutral', 'working', 'attention'].map(t =>
    `<td><span class="wk-dot" data-size="sm" data-shape="${s}" data-tone="${t}" aria-hidden="true"></span><span class="wk-dot" data-shape="${s}" data-tone="${t}" role="img" aria-label="${s} ${t}"></span><span class="wk-dot g-dot-large" data-shape="${s}" data-tone="${t}" aria-hidden="true"></span></td>`).join('')}</tr>`).join('')}</tbody>
</table>`,
    }, {
      title: 'Dot label',
      note: 'A small dot and its words, as one line.',
      html: `<div class="g-row">
  <span class="wk-dot-label"><span class="wk-dot" data-size="sm" aria-hidden="true"></span><span>Connected</span></span>
  <span class="wk-dot-label"><span class="wk-dot" data-size="sm" data-shape="hollow" aria-hidden="true"></span><span>Reconnecting</span></span>
  <span class="wk-dot-label"><span class="wk-dot" data-size="sm" data-tone="attention" aria-hidden="true"></span><span>Needs you</span></span>
</div>`,
    }],
  },
  {
    id: 'motion', title: 'Motion and timings', group: 'foundations',
    intro: 'The action lifecycle\'s timings (<code>timings</code>) and the motion tokens (<code>motion</code>, <code>--wk-duration-*</code>, <code>--wk-ease-*</code>), read from the kit this page loaded. BEHAVIOUR.md is the spec. With reduced motion, transitions are instant or a fade of 100 ms at most.',
    examples: [{
      title: 'Timings and motion values',
      html: '<table class="wk-table g-timings" id="g-timings"><thead><tr><th>Name</th><th class="wk-num">Value</th></tr></thead><tbody></tbody></table><p class="wk-caption" id="g-reduced"></p>',
      after(el, { kit }) {
        const body = el.querySelector('tbody');
        const value = v => (Array.isArray(v) ? v.map(n => `${n / 1000} s`).join(', ') : typeof v === 'number' ? (v >= 1000 ? `${v / 1000} s` : v < 1 ? `±${v * 100}%` : `${v} ms`) : v);
        for (const [group, values] of [['timings', kit.timings], ['motion', kit.motion]]) {
          for (const [name, v] of Object.entries(values)) {
            const row = body.insertRow();
            const th = document.createElement('th');
            th.scope = 'row';
            th.innerHTML = `<code></code>`;
            th.firstChild.textContent = `${group}.${name}`;
            const td = document.createElement('td');
            td.className = 'wk-num';
            td.textContent = value(v);
            row.append(th, td);
          }
        }
        el.querySelector('#g-reduced').textContent = kit.reducedMotion() ? 'Reduced motion is on.' : 'Reduced motion is off (the system setting; DevTools can emulate it).';
      },
    }],
  },
];
