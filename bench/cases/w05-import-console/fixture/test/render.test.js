import test from 'node:test';
import assert from 'node:assert/strict';
import {
  classNames,
  escapeAttr,
  escapeDeep,
  escapeText,
  escapeUrl,
  htmlAttributes,
  jsonForScript,
  stripTags,
} from '../src/render/escape.js';
import { layout, renderBreadcrumbs, renderFlash, renderNav, renderPager, renderTableOfContents } from '../src/render/layout.js';
import { renderTable, renderKeyValue, renderRowActions, columnWidths } from '../src/render/components/table.js';
import { renderField, renderForm, renderHidden, renderUploadForm } from '../src/render/components/form.js';
import { renderCard, renderProfileCard, renderStatCard } from '../src/render/components/card.js';
import { renderBars, renderLegend, renderSparkline } from '../src/render/components/chart.js';

test('escapeText escapes markup but leaves quotes for text nodes', () => {
  assert.equal(escapeText('<b>a & b</b>'), '&lt;b&gt;a &amp; b&lt;/b&gt;');
  assert.equal(escapeText('plain "quoted"'), 'plain "quoted"');
  assert.equal(escapeText(undefined), '');
});

test('escapeAttr escapes quotes so an attribute cannot be closed', () => {
  assert.equal(escapeAttr('a"b'), 'a&quot;b');
  assert.equal(escapeAttr("a'b"), 'a&#39;b');
  assert.equal(escapeAttr('a`b'), 'a&#96;b');
  assert.equal(escapeAttr('<x>'), '&lt;x&gt;');
});

test('escapeUrl only admits the schemes the console uses', () => {
  assert.equal(escapeUrl('https://example.com/a?b=1'), 'https://example.com/a?b=1');
  assert.equal(escapeUrl('/reports/items'), '/reports/items');
  assert.equal(escapeUrl('mailto:ops@example.com'), 'mailto:ops@example.com');
  assert.equal(escapeUrl('javascript:alert(1)'), '#');
  assert.equal(escapeUrl('data:text/html,x'), '#');
  assert.equal(escapeUrl(''), '#');
});

test('helper escapes compose', () => {
  assert.equal(stripTags('<p>hello</p>'), 'hello');
  assert.equal(jsonForScript({ a: '</script>' }), '{"a":"\\u003c/script>"}');
  assert.equal(classNames('a', ['b', ''], undefined), 'a b');
  assert.deepEqual(escapeDeep({ a: '<b>' }), { a: '&lt;b&gt;' });
  assert.equal(htmlAttributes({ class: 'x', hidden: true, title: 'a"b', skip: undefined }), 'class="x" hidden title="a&quot;b"');
});

test('the layout escapes the title and the page data', () => {
  const page = layout({
    title: '<script>alert(1)</script>',
    body: '<p>ok</p>',
    pageData: { term: '</script>' },
    user: { name: 'Sam <admin>', email: 'sam"@example.com' },
  });
  assert.match(page, /<title>&lt;script&gt;alert\(1\)&lt;\/script&gt;<\/title>/);
  assert.equal(page.includes('<script>alert(1)</script>'), false);
  assert.match(page, /window\.page = \{"term":"\\u003c\/script>"\}/);
  assert.match(page, /data-email="sam&quot;@example.com"/);
});

test('navigation and breadcrumbs escape their links', () => {
  const nav = renderNav('/items');
  assert.match(nav, /href="\/items" aria-current="page"/);
  const crumbs = renderBreadcrumbs([
    { label: 'Home', href: '/?a="b' },
    { label: 'Items' },
  ]);
  assert.match(crumbs, /href="\/\?a=&quot;b"/);
  assert.match(crumbs, /aria-current="page">Items/);
});

test('flash messages and pagers escape their contents', () => {
  const flash = renderFlash([{ kind: 'error', text: '<b>bad</b>' }]);
  assert.match(flash, /&lt;b&gt;bad&lt;\/b&gt;/);
  const pager = renderPager(2, 4, '/items');
  assert.match(pager, /href="\/items\?page=1"/);
  assert.match(pager, /2 of 4/);
});

test('the table of contents anchors are plain', () => {
  const toc = renderTableOfContents([{ label: 'Rows', id: 'a b<c' }]);
  assert.match(toc, /href="#a-b-c"/);
});

test('the document page keeps a benign href and escapes attribute context', () => {
  const html = renderTable({
    caption: 'Rows "quoted"',
    columns: [
      { key: 'sku', label: 'SKU' },
      { key: 'site', label: 'Site', link: true },
    ],
    rows: [{ sku: '<SKU-1>', site: 'https://example.com/a?b=1' }],
  });
  assert.match(html, /data-caption="Rows &quot;quoted&quot;"/);
  assert.match(html, /<td>&lt;SKU-1&gt;<\/td>/);
  assert.match(html, /href="https:\/\/example.com\/a\?b=1"/);
});

test('money cells are formatted and rows can be empty', () => {
  const html = renderTable({
    columns: [{ key: 'amount', label: 'Amount', money: true }],
    rows: [{ amount: 123456 }],
    currency: 'USD',
  });
  assert.match(html, /\$1,234.56/);
  assert.match(renderTable({ columns: [{ key: 'a', label: 'A' }], rows: [], caption: 'none' }), /<tbody><\/tbody>/);
});

test('key value tables and action links escape both contexts', () => {
  const html = renderKeyValue([{ label: '<Total>', value: 'x & y', money: false }]);
  assert.match(html, /&lt;Total&gt;/);
  assert.match(html, /x &amp; y/);
  const actions = renderRowActions({ id: 'a b' }, [{ base: '/items', label: 'Remove' }]);
  assert.match(actions, /href="\/items\/a%20b"/);
  assert.equal(columnWidths([{ key: 'a', label: 'A' }], [{ a: 'longer value' }])[0], 12);
});

test('form fields escape values and attribute names', () => {
  const field = renderField({ name: 'title', label: 'Title', value: 'a"b', hint: 'Use <plain> text' });
  assert.match(field, /value="a&quot;b"/);
  assert.match(field, /Use &lt;plain&gt; text/);
  const select = renderField({
    name: 'status',
    label: 'Status',
    type: 'select',
    value: 'paid',
    options: [{ value: 'paid', label: 'Paid' }, { value: 'open', label: 'Open' }],
  });
  assert.match(select, /<option value="paid" selected>Paid<\/option>/);
  const textarea = renderField({ name: 'note', type: 'textarea', value: '<b>x</b>' });
  assert.match(textarea, /&lt;b&gt;x&lt;\/b&gt;/);
});

test('forms and uploads use the field renderer', () => {
  const form = renderForm({
    action: '/items?q="x',
    fields: [{ name: 'sku', label: 'SKU', required: true }],
    errors: [{ field: 'sku', message: 'is required' }],
  });
  assert.match(form, /action="\/items\?q=&quot;x"/);
  assert.match(form, /required/);
  assert.match(form, /data-field="sku"/);
  const upload = renderUploadForm({ datasets: [{ value: 'items', label: 'Items' }] });
  assert.match(upload, /enctype="multipart\/form-data"/);
  assert.match(renderHidden('next', '/a "b"'), /value="\/a &quot;b&quot;"/);
});

test('cards use escapeAttr for their href and keep the visible text escaped', () => {
  const card = renderCard({ title: '<b>Items</b>', body: 'a & b', href: '/items?q="x' });
  assert.match(card, /<h2>&lt;b&gt;Items&lt;\/b&gt;<\/h2>/);
  assert.match(card, /href="\/items\?q=&quot;x"/);
  const profile = renderProfileCard({ name: 'Sam', email: 'a"b@example.com', roles: ['<admin>'] });
  assert.match(profile, /href="mailto:a&quot;b@example.com"/);
  assert.match(profile, /&lt;admin&gt;/);
});

test('stat cards format deltas', () => {
  const card = renderStatCard({ label: 'Revenue', value: 123456, money: true, delta: 12.34, trend: 'up' });
  assert.match(card, /\$1,234.56/);
  assert.match(card, /\+12\.3%/);
  assert.match(card, /data-trend="up"/);
});

test('charts escape every attribute and label', () => {
  const svg = renderBars({
    data: [
      { label: 'a"b', value: 3 },
      { label: '<c>', value: 5 },
    ],
    title: 'Sales & <margins>',
    width: 200,
  });
  assert.match(svg, /data-label="a&quot;b"/);
  assert.match(svg, /Sales &amp; &lt;margins&gt;/);
  assert.match(svg, />a"b<\/text>/);
  const spark = renderSparkline([1, 4, 2]);
  assert.match(spark, /<polyline points="0\.0,23\.0/);
  assert.match(renderLegend([{ key: 'a"b', label: '<A>' }]), /data-key="a&quot;b"/);
});
