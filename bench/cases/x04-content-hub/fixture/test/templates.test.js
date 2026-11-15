import test from 'node:test';
import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { escapeText, escapeAttr, escapeJson, safeUrl, tag, attrs, unescapeText } from '../src/templates/escaping.js';
import { parseTemplate, renderTemplate, variablesOf, countNodes, stripComments, assertBalanced, inlinePartials } from '../src/templates/engine.js';
import { helperMap, registerHelper, listHelpers, composeHelpers } from '../src/templates/helpers.js';
import { TemplateLoader } from '../src/templates/loader.js';
import { PartialRegistry, partialName, renderBreadcrumbs, renderPagination } from '../src/templates/partials.js';
import { ThemeRegistry, themeDefaults, DEFAULT_THEME } from '../src/templates/themes.js';
import { tempRoots, cleanup } from './helpers.js';

test('escaping separates text and attribute contexts', () => {
  assert.equal(escapeText('<b>"x"</b>'), '&lt;b&gt;&quot;x&quot;&lt;/b&gt;');
  assert.equal(escapeAttr('<b>"x"</b>'), '&lt;b&gt;&quot;x&quot;&lt;/b&gt;');
  assert.equal(escapeAttr('a\u0000b'), 'ab');
  assert.equal(unescapeText('&lt;p&gt;'), '<p>');
  assert.equal(escapeJson('</script>'), '"\\u003c/script\\u003e"');
  assert.equal(safeUrl('javascript:alert(1)'), '');
  assert.equal(safeUrl('https://example.invalid/x'), 'https://example.invalid/x');
  assert.equal(tag('p', { class: 'note' }, 'hi'), '<p class="note">hi</p>');
  assert.equal(attrs({ a: 1, b: undefined }), ' a="1"');
});

test('templates render variables, blocks and helpers', () => {
  const template = parseTemplate('<h1>{{title}}</h1>{{#each items}}<li>{{name}}</li>{{/each}}');
  const html = renderTemplate(template, { title: 'Home', items: [{ name: 'a' }, { name: 'b' }] }, helperMap());
  assert.equal(html, '<h1>Home</h1><li>a</li><li>b</li>');
  assert.equal(renderTemplate(parseTemplate('{{{raw}}}'), { raw: '<b>' }, helperMap()), '<b>');
  assert.deepEqual([...variablesOf(template.nodes)].sort(), ['items', 'name', 'title']);
  assert.equal(countNodes(template.nodes) > 0, true);
  assert.equal(assertBalanced('{{#if a}}x{{/if}}'), true);
  assert.throws(() => assertBalanced('{{#if a}}x'), /open/);
});

test('template helpers format values', () => {
  const helpers = helperMap();
  assert.equal(renderTemplate(parseTemplate('{{upper name}}'), { name: 'sam' }, helpers), 'SAM');
  assert.equal(renderTemplate(parseTemplate('{{slug title}}'), { title: 'Hello World' }, helpers), 'hello-world');
  assert.equal(renderTemplate(parseTemplate('{{json payload}}'), { payload: { a: 1 } }, helpers), '{"a":1}');
  const extended = registerHelper(helpers, 'shout', (value, scope, render) => render([{ kind: 'text', value: `${value}!` }], scope));
  assert.equal(renderTemplate(parseTemplate('{{shout name}}'), { name: 'hi' }, extended), 'hi!');
  assert.ok(listHelpers(helpers).includes('escape'));
  assert.ok(composeHelpers(helpers, { extra: () => '' }).extra);
});

test('the loader reads templates under its root', async () => {
  const roots = await tempRoots();
  try {
    await writeFile(join(roots.templates, 'entry.html'), '<h1>{{title}}</h1>');
    const loader = new TemplateLoader({ root: roots.templates, helpers: helperMap() });
    const template = await loader.load('entry.html');
    assert.equal(template.name, 'entry.html');
    assert.equal(renderTemplate(template, { title: 'Hello' }, helperMap()), '<h1>Hello</h1>');
    assert.deepEqual(await loader.list(), ['entry.html']);
    assert.equal(await loader.exists('missing.html'), false);
    assert.equal((await loader.sources())['entry.html'], '<h1>{{title}}</h1>');
    await assert.rejects(() => loader.load('../secret.html'), /unsafe path/);
    assert.equal(loader.invalidate('entry.html'), 1);
  } finally {
    await cleanup(roots);
  }
});

test('partials are registered and rendered', async () => {
  const registry = new PartialRegistry();
  registry.define('card', '<div>{{title}}</div>');
  assert.equal(registry.has('card'), true);
  assert.equal(partialName('card.html'), 'card');
  assert.equal(registry.names().length, 1);
  const html = renderBreadcrumbs([{ label: 'Home' }, { label: 'Docs' }]);
  assert.match(html, /<ul class="breadcrumbs"/);
  assert.match(renderPagination({ total: 40, limit: 20, offset: 0 }), /data-pages="2"/);
});

test('themes list their templates', () => {
  const themes = new ThemeRegistry(themeDefaults());
  assert.equal(themes.stats().themes, 2);
  assert.equal(themes.current().name, DEFAULT_THEME);
  assert.equal(themes.activate('print').name, 'print');
  assert.equal(themes.resolveTemplate('print', 'entry.html'), 'entry.html');
  assert.throws(() => themes.resolveTemplate('print', 'article-card.html'), /not found/);
  assert.match(themes.cssVariables('print'), /--accent/);
  assert.equal(themes.list().length, 2);
});

test('comments are stripped before parsing', () => {
  assert.equal(stripComments('a{{!-- hidden --}}b'), 'ab');
  assert.deepEqual(inlinePartials(parseTemplate('{{#partial "card"}}x{{/partial}}').nodes), ['card']);
});
