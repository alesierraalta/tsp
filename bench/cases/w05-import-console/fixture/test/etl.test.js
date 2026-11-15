import test from 'node:test';
import assert from 'node:assert/strict';
import {
  detectDelimiter,
  normalizeNewlines,
  parseCsv,
  parseTsv,
  recordCount,
  sampleLine,
  splitRecords,
} from '../src/etl/reader.js';
import {
  DATASETS,
  columnOf,
  columnSummary,
  compareSchemas,
  coerce,
  isEmail,
  keyColumns,
  optionalColumns,
  requiredColumns,
  schemaOf,
} from '../src/etl/schema.js';
import { countProblems, errorReport, firstProblem, problemsByColumn, validateRow, validateRows } from '../src/etl/validate.js';
import {
  blankToNull,
  canonicalHeaders,
  centsColumns,
  mergeDuplicate,
  normalizeEmail,
  normalizeMoney,
  normalizeRow,
  renameColumns,
} from '../src/etl/normalize.js';
import { Dataset } from '../src/etl/store.js';
import { compatibleInput, datasetNames, importDataset, loadDataset, preview } from '../src/etl/loader.js';
import { ImportError } from '../src/util/errors.js';
import { canonicalKey, canonicalHeader } from '../src/util/text.js';

const ITEMS_CSV = [
  'sku,title,category,amount,stock,updated_at',
  'SKU-1,Desk lamp,lighting,120.50,4,2026-01-05',
  'SKU-2,Chair,seating,89.00,12,2026-01-06',
].join('\n');

test('parseCsv reads quotes, embedded commas and CRLF', () => {
  const parsed = parseCsv('sku,title\r\nA1,"Lamp, large"\r\nA2,"Say ""hi"""\r\n');
  assert.deepEqual(parsed.header, ['sku', 'title']);
  assert.deepEqual(parsed.rows, [
    { sku: 'A1', title: 'Lamp, large' },
    { sku: 'A2', title: 'Say "hi"' },
  ]);
});

test('parseCsv keeps a newline inside a quoted field', () => {
  const parsed = parseCsv('sku,note\nA1,"line one\nline two"\n');
  assert.equal(parsed.rows[0].note, 'line one\nline two');
});

test('a record with the wrong field count is refused', () => {
  assert.throws(() => parseCsv('a,b\n1,2,3\n'), /3 fields but the header names 2/);
});

test('delimiters are detected and rows counted', () => {
  assert.equal(detectDelimiter('a;b;c\n1;2;3'), ';');
  assert.equal(detectDelimiter('a\tb\n1\t2'), '\t');
  assert.equal(detectDelimiter('a,b\n1,2'), ',');
  assert.equal(recordCount('a,b\n1,2\n3,4', ','), 2);
  assert.equal(sampleLine('a,b\n1,2', 1), '1,2');
  assert.equal(normalizeNewlines('a\r\nb\rc'), 'a\nb\nc');
  assert.equal(parseTsv('a\tb\n1\t2').rows[0].b, '2');
  assert.deepEqual(splitRecords('a,"b\nc"', ','), [['a', 'b\nc']]);
});

test('schemas name their columns and keys', () => {
  const items = schemaOf('items');
  assert.deepEqual(keyColumns(items), ['sku']);
  assert.deepEqual(requiredColumns(items), ['sku', 'title', 'amount']);
  assert.equal(optionalColumns(items).includes('stock'), true);
  assert.equal(columnOf(items, 'amount').type, 'money');
  assert.throws(() => columnOf(items, 'nope'), (error) => error.code === 'schema_error');
  assert.throws(() => schemaOf('nope'), /unknown dataset/);
  assert.deepEqual(datasetNames(), ['customers', 'items', 'orders']);
  assert.equal(columnSummary(items)[0], 'sku:text!');
});

test('coerce and isEmail follow the column type', () => {
  assert.equal(coerce('integer', ' 12 '), 12);
  assert.equal(coerce('integer', 'nope'), null);
  assert.equal(coerce('enum', 'PAID'), 'paid');
  assert.equal(coerce('email', ' A@B.CO '), 'a@b.co');
  assert.equal(coerce('text', '   '), null);
  assert.equal(isEmail('a@b.co'), true);
  assert.equal(isEmail('nope'), false);
  const compared = compareSchemas(schemaOf('items'), { columns: [{ name: 'sku' }] });
  assert.equal(compared.compatible, false);
  assert.equal(compared.missing.includes('amount'), true);
});

test('validateRow names each problem', () => {
  const schema = schemaOf('orders');
  const ok = validateRow(
    schema,
    { order_id: 'O-1', customer: 'a@b.co', total: '10.00', status: 'paid', placed_at: '2026-01-05' },
    2,
  );
  assert.deepEqual(ok.problems, []);
  const bad = validateRow(
    schema,
    { order_id: 'O-1', customer: 'nope', total: 'ten', status: 'lost', placed_at: '2026-13-01' },
    3,
  );
  assert.equal(bad.problems.length, 4);
  assert.deepEqual(
    bad.problems.map((item) => item.field),
    ['customer', 'total', 'status', 'placed_at'],
  );
  const missing = validateRow(schema, { order_id: '' }, 4);
  assert.equal(missing.problems.some((item) => item.message === 'is required'), true);
});

test('validateRows reports unknown columns and totals', () => {
  const schema = schemaOf('items');
  const rows = [
    { sku: 'A1', title: 'Lamp', amount: '1.00', extra: 'x' },
    { sku: '', title: '', amount: 'nope' },
  ];
  const validation = validateRows(schema, rows);
  assert.equal(validation.ok, false);
  assert.equal(validation.errors.length, 1);
  assert.deepEqual(validation.unknown, [{ row: 2, name: 'extra' }]);
  assert.equal(countProblems(validation), 3);
  assert.equal(firstProblem(validation).field, 'sku');
  assert.deepEqual(Object.keys(problemsByColumn(validation)).sort(), ['amount', 'sku', 'title']);
  assert.match(errorReport(validation), /unknown columns: extra/);
  assert.equal(validateRows(schema, rows, { allowUnknown: true }).unknown.length, 1);
});

test('headers are canonicalized and duplicates numbered', () => {
  assert.equal(canonicalHeader('  Total Amount '), 'total_amount');
  assert.deepEqual(canonicalHeaders(['SKU', 'Total Amount', 'total amount']), [
    'sku',
    'total_amount',
    'total_amount_1',
  ]);
  const schema = schemaOf('items');
  const renamed = renameColumns(
    [{ SKU: 'A1', Title: 'Lamp', Amount: '1.00', Unknown: 'x' }],
    ['SKU', 'Title', 'Amount', 'Unknown'],
    schema,
  );
  assert.deepEqual(renamed[0], { sku: 'A1', title: 'Lamp', amount: '1.00' });
});

test('normalization converts blanks and money to cents', () => {
  const schema = schemaOf('items');
  assert.deepEqual(blankToNull({ a: '  ', b: 'x' }), { a: null, b: 'x' });
  const row = normalizeRow(schema, { sku: 'A1', title: 'Lamp', amount: '120.50', stock: '4' });
  assert.equal(row.stock, 4);
  assert.deepEqual(centsColumns(schema), ['amount']);
  const money = normalizeMoney(schema, [row]);
  assert.equal(money[0].amount, 12050);
  assert.equal(normalizeEmail(' A@B.CO '), 'a@b.co');
  const merged = mergeDuplicate(
    [{ sku: 'A1', amount: 1 }, { sku: 'a1', amount: 2 }, { sku: 'B2', amount: 3 }],
    ['sku'],
    (left, right) => ({ ...left, amount: left.amount + right.amount }),
  );
  assert.equal(merged.length, 2);
  assert.equal(merged[0].amount, 3);
});

test('the dataset indexes rows by their canonical key', () => {
  const dataset = new Dataset(schemaOf('items'));
  dataset.replaceAll([
    { sku: 'A1', title: 'Lamp', amount: 100 },
    { sku: 'B2', title: 'Chair', amount: 250 },
  ]);
  assert.equal(dataset.count(), 2);
  assert.equal(dataset.get('a1').title, 'Lamp');
  assert.equal(canonicalKey('A1'), 'a1');
  assert.equal(dataset.total('amount'), 350);
  assert.deepEqual(dataset.totals(), { amount: 350 });
  assert.equal(dataset.columns().length, 6);
  assert.throws(() => dataset.add({ sku: 'a1', title: 'Dup', amount: 1 }), /duplicate key/);
  dataset.upsert({ sku: 'a1', title: 'Lamp v2', amount: 120 });
  assert.equal(dataset.get('A1').title, 'Lamp v2');
  assert.equal(dataset.remove('A1'), true);
  assert.equal(dataset.remove('A1'), false);
  assert.equal(dataset.count(), 1);
});

test('the dataset filters, sorts, groups and pages', () => {
  const dataset = new Dataset(schemaOf('orders'));
  dataset.replaceAll([
    { order_id: 'O-1', customer: 'a@b.co', total: 500, status: 'paid', placed_at: '2026-01-05' },
    { order_id: 'O-2', customer: 'c@d.co', total: 900, status: 'paid', placed_at: '2026-02-05' },
    { order_id: 'O-3', customer: 'e@f.co', total: 100, status: 'pending', placed_at: '2026-02-06' },
  ]);
  assert.equal(dataset.filter([{ op: 'eq', column: 'status', value: 'PAID' }]).length, 2);
  assert.equal(dataset.filter([{ op: 'gte', column: 'total', value: 500 }]).length, 2);
  assert.equal(dataset.filter([{ op: 'lte', column: 'total', value: 500 }]).length, 2);
  assert.equal(dataset.filter([{ op: 'search', column: 'customer', value: '@D.CO' }]).length, 1);
  dataset.sortBy('total', 'desc');
  assert.equal(dataset.all()[0].order_id, 'O-2');
  assert.deepEqual(
    dataset.groupSummary('status', { money: 'total' }).map((entry) => [entry.label, entry.count, entry.total]),
    [
      ['paid', 2, 1400],
      ['pending', 1, 100],
    ],
  );
  assert.equal(dataset.page(1, 1).length, 1);
  assert.equal(dataset.page(10, 0).length, 3);
  dataset.clear();
  assert.equal(dataset.count(), 0);
});

test('loadDataset imports the sample file with money in cents', () => {
  const result = loadDataset('items', ITEMS_CSV);
  assert.equal(result.dataset.count(), 2);
  assert.equal(result.dataset.get('SKU-1').amount, 12050);
  assert.deepEqual(result.validation.errors, []);
  assert.equal(result.delimiter, ',');
});

test('loadDataset refuses a file with a bad row', () => {
  const csv = 'sku,title,amount\nA1,,nope\n';
  assert.throws(() => loadDataset('items', csv), (error) => {
    assert.ok(error instanceof ImportError);
    assert.equal(error.code, 'invalid_row');
    assert.equal(error.details.row, 2);
    return true;
  });
});

test('loadDataset refuses more rows than the limit', () => {
  const csv = `sku,title,amount\n${Array.from({ length: 5 }, (_, index) => `S${index},T,1.00`).join('\n')}`;
  assert.throws(() => loadDataset('items', csv, { limit: 2 }), /more than 2 rows/);
});

test('importDataset, preview and compatibility helpers', () => {
  const imported = importDataset('items', ITEMS_CSV);
  assert.equal(imported.rows, 2);
  assert.equal(imported.key[0], 'sku');
  assert.equal(typeof imported.checksum, 'string');
  const sample = preview(ITEMS_CSV, { dataset: 'items', rows: 1 });
  assert.equal(sample.rowCount, 2);
  assert.equal(sample.sample.length, 1);
  assert.deepEqual(compatibleInput('items', ['sku', 'title']).missing, [
    'category',
    'amount',
    'stock',
    'updated_at',
  ]);
});

test('every shipped dataset declares a key', () => {
  for (const [name, schema] of Object.entries(DATASETS)) {
    assert.equal(schema.name, name);
    assert.ok(schema.key.length > 0);
  }
});
