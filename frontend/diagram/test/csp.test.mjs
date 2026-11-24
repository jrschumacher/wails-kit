// The checks src/mermaid/csp.js uses to keep a diagram from loading
// anything: a url() is local only when its whole argument is one fragment.
import test from 'node:test';
import assert from 'node:assert/strict';
import { externalUrl, loadsInAttribute, dropLoading } from '../src/mermaid/csp.js';

test('a url() is local only when its argument is a single fragment token', () => {
  for (const local of ['url(#a)', 'url( #arrow-end )', 'url("#m")', "url('#m')", 'none', 'red', 'url(#a) url(#b)']) assert.equal(externalUrl(local), false, local);
  for (const ext of ['url(x.svg#a)', 'url(//evil.example/f.svg#a)', 'url("#" //evil.example/x)', 'url(# //evil)', 'url("#a" x)', "url('#a\")", 'url(#a) url(b.png)', 'URL(b)', 'url(', 'url("#a', 'url(#a"']) assert.equal(externalUrl(ext), true, ext);
});

test('presentation attribute values: external url(), image-set() and CSS escapes load', () => {
  assert.equal(loadsInAttribute('url(#arrowhead)'), false);
  assert.equal(loadsInAttribute('#333'), false);
  for (const v of ['url("#" //evil.example/x)', 'url(rel.svg#f)', 'image-set("a.png" 1x)', 'u\\72l(x)']) assert.equal(loadsInAttribute(v), true, v);
});

test('a style declaration that loads something is dropped whole', () => {
  assert.equal(dropLoading('fill:#f9f; mask: url("#" //evil.example/m); stroke: url(#ok)'), 'fill:#f9f; stroke: url(#ok)');
  assert.equal(dropLoading('font-family: "a;b"; background: url(x)'), 'font-family: "a;b"');
});
