import test from 'node:test';
import assert from 'node:assert/strict';
import { createPipeline, SAMPLE_ITEMS } from '../src/pipeline.js';
import { datasetLimit, describeConfig, isMoneyColumn, loadConfig, rowBudget, withOverrides } from '../src/config.js';
import { renderPage, pageNames, renderNotFound, renderErrorPage } from '../src/render/pages/index.js';
import { layout } from '../src/render/layout.js';
import { escapeAttr } from '../src/render/escape.js';
import { renderTable } from '../src/render/components/table.js';
import { renderCard } from '../src/render/components/card.js';
import { ImportError } from '../src/util/errors.js';

function pipeline(options = {}) {
  return createPipeline(options);
}

test('configuration defaults and limits', () => {
  const config = loadConfig();
  assert.equal(config.currency, 'USD');
  assert.equal(config.pageSize, 25);
  assert.equal(datasetLimit(config, '1000'), 100);
  assert.equal(datasetLimit(config, '5'), 5);
  assert.equal(datasetLimit(config, 'nope'), 25);
  assert.equal(rowBudget(config), 5000);
  assert.match(describeConfig(config), /currency=USD/);
  assert.equal(isMoneyColumn(config, 'Amount'), true);
  assert.equal(isMoneyColumn(config, 'title'), false);
});

test('configuration refuses unknown keys and bad values', () => {
  assert.throws(() => loadConfig({ nope: 1 }), /unknown configuration key/);
  assert.throws(() => loadConfig({ title: '' }), /non empty/);
  assert.throws(() => loadConfig({ pageSize: 500 }), /pageSize/);
  assert.throws(() => loadConfig({ currency: 'XYZ' }), /unsupported currency/);
  assert.throws(() => loadConfig({ dateFormat: 'wide' }), /unsupported date format/);
  assert.throws(() => loadConfig({ moneyColumns: [1] }), /column names/);
  assert.equal(withOverrides(loadConfig(), { pageSize: 10 }).pageSize, 10);
});

test('the pipeline imports, exports and reports the sample data', () => {
  const console = pipeline({ now: Date.UTC(2026, 2, 1) });
  const imported = console.importCsv('items', SAMPLE_ITEMS);
  assert.equal(imported.rows, 2);
  assert.deepEqual(console.names(), ['items']);
  assert.equal(console.history.length, 1);
  assert.equal(console.history[0].rows, 2);

  const summary = console.summary('items');
  assert.equal(summary.totalCents, 20950);
  const csv = console.exportTable('items', 'csv');
  assert.match(csv, /^sku,title/);
  assert.match(csv, /SKU-1,Desk lamp/);
  assert.match(console.exportTable('items', 'tsv'), /\t/);
  assert.throws(() => console.exportTable('items', 'xml'), /unknown table format/);
  assert.match(console.exportSql('items'), /^SELECT "sku"/);
  assert.match(console.exportInserts('items'), /INSERT INTO "items"/);
  assert.equal(console.manifest('items').rows, 2);
  assert.match(console.describe('items'), /items rows=2/);
  assert.equal(console.statement('items').totals.totalCents, 20950);
  assert.equal(console.estimate('items', { shipping: 'pickup' }).subtotalCents, 20950);
});

test('the pipeline names the dataset it does not hold', () => {
  const console = pipeline();
  assert.throws(() => console.datasetOf('orders'), /has not been imported/);
  assert.throws(() => console.importCsv('items', 'sku,title\nA,\n'), (error) => {
    assert.ok(error instanceof ImportError);
    return true;
  });
});

test('the dashboard page renders its cards and totals', () => {
  const html = renderPage('dashboard', {
    imports: [{ rows: 2 }],
    items: [{}, {}],
    orders: [],
    itemCount: 2,
    openOrders: 0,
    revenueCents: 123456,
    revenueByMonth: [{ label: '2026-01', value: 1234 }],
    ordersByStatus: [{ label: 'paid', value: 2 }],
    events: [{ when: '2026-01-05', what: 'Import', who: 'sam' }],
  });
  assert.match(html, /<title>Dashboard<\/title>/);
  assert.match(html, /\$1,234\.56/);
  assert.match(html, /Revenue by month/);
  assert.match(html, /Import/);
  assert.equal(html.includes('window.page'), true);
});

test('an empty dashboard and the unknown page render', () => {
  const empty = renderPage('dashboard', { imports: [] });
  assert.match(empty, /Nothing imported yet/);
  const missing = renderNotFound('No such page');
  assert.match(missing, /<title>Not found<\/title>/);
  assert.match(missing, /No such page/);
  const error = renderErrorPage(new Error('boom'));
  assert.match(error, /Something went wrong/);
  assert.match(error, /boom/);
  assert.deepEqual(pageNames(), ['dashboard', 'imports', 'notFound', 'report', 'result', 'validation']);
});

test('the report page renders money and key values', () => {
  const html = renderPage('report', {
    dataset: 'items',
    rows: [{ sku: 'A1', amount: 12050 }],
    columns: [{ key: 'sku', label: 'SKU' }, { key: 'amount', label: 'Amount', money: true }],
    total: 1,
    totalCents: 12050,
    pages: 1,
    currency: 'USD',
    caption: 'Rows',
    generatedAt: '2026-01-01',
    checksum: 'abc',
  });
  assert.match(html, /<title>Report: items<\/title>/);
  assert.match(html, /\$120\.50/);
  assert.match(html, /Checksum/);
});

test('the layout is the shared shell for every page', () => {
  const html = layout({ title: 'Title', body: '<p>body</p>', messages: [{ kind: 'info', text: 'hi' }] });
  assert.match(html, /<h1>Title<\/h1>/);
  assert.match(html, /<p>body<\/p>/);
  assert.match(html, /class="flash info"/);
});

test('a rendered table keeps escaped text and attributes apart', () => {
  const html = renderTable({
    caption: 'A "caption"',
    columns: [
      { key: 'title', label: 'Title' },
      { key: 'site', label: 'Site', link: true },
    ],
    rows: [{ title: '<Lamp>', site: 'https://example.com/lamp' }],
  });
  assert.match(html, /data-caption="A &quot;caption&quot;"/);
  assert.match(html, /&lt;Lamp&gt;/);
  assert.match(html, new RegExp(`href="${escapeAttr('https://example.com/lamp')}"`));
  assert.equal(renderCard({ title: 'Card', body: 'Body' }).includes('<h2>Card</h2>'), true);
});
