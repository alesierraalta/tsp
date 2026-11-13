import test from 'node:test';
import assert from 'node:assert/strict';
import {
  bulkInsertQuery,
  deleteQuery,
  insertQuery,
  limitClause,
  orderClause,
  quoteIdent,
  quoteLiteral,
  searchClause,
  selectQuery,
  updateQuery,
  whereClause,
  whereClauses,
} from '../src/query/builder.js';
import {
  exportFiltered,
  exportInsert,
  exportSearch,
  exportSelect,
  exportUpsert,
  selectStatement,
  statementKind,
  statementsFor,
} from '../src/export/sql.js';
import {
  columnNames,
  csvChunks,
  escapeCell,
  fileNameFor,
  parseCsvLine,
  rowCount,
  toCsv,
  toCsvByColumnTypes,
  toTsv,
  withBom,
} from '../src/export/csv.js';
import { compareManifests, describeManifest, manifestFor, manifestLines } from '../src/export/manifest.js';
import { loadDataset } from '../src/etl/loader.js';

const CSV = [
  'sku,title,category,amount,stock,updated_at',
  'SKU-1,Desk lamp,lighting,120.50,4,2026-01-05',
  'SKU-2,Chair,seating,89.00,12,2026-01-06',
].join('\n');

function dataset() {
  return loadDataset('items', CSV).dataset;
}

test('quoteIdent accepts plain names only', () => {
  assert.equal(quoteIdent('items'), '"items"');
  assert.equal(quoteIdent('total_amount'), '"total_amount"');
  assert.throws(() => quoteIdent('items; DROP TABLE x'), /plain identifier/);
  assert.throws(() => quoteIdent('a"b'), /plain identifier/);
});

test('quoteLiteral doubles quotes and keeps other types bare', () => {
  assert.equal(quoteLiteral('acme'), "'acme'");
  assert.equal(quoteLiteral("x' OR '1'='1"), "'x'' OR ''1''=''1'");
  assert.equal(quoteLiteral(42), '42');
  assert.equal(quoteLiteral(true), 'TRUE');
  assert.equal(quoteLiteral(null), 'NULL');
  assert.equal(quoteLiteral(undefined), 'NULL');
  assert.throws(() => quoteLiteral(Number.NaN), /finite/);
});

test('whereClause quotes every literal it writes', () => {
  assert.equal(whereClause({ column: 'sku', value: "A'1" }), `"sku" = 'A''1'`);
  assert.equal(whereClause({ column: 'total', op: 'gte', value: 100 }), '"total" >= 100');
  assert.equal(whereClause({ column: 'sku', op: 'in', value: ['A', "B'C"] }), `"sku" IN ('A', 'B''C')`);
  assert.equal(
    whereClause({ column: 'title', op: 'search', value: "x' OR '1'='1" }),
    `LOWER("title") LIKE '%x'' OR ''1''=''1%'`,
  );
  assert.equal(searchClause('title', "a'b"), `LOWER("title") LIKE '%a''b%'`);
  assert.throws(() => whereClause({ column: 'sku', op: 'nope', value: 1 }), /unknown filter op/);
  assert.throws(() => whereClause({ column: 'sku', op: 'in', value: [] }), /at least one value/);
  assert.equal(whereClauses([]).length, 0);
});

test('order, limit and select clauses are built in order', () => {
  assert.equal(orderClause({ column: 'sku', dir: 'desc' }), 'ORDER BY "sku" DESC');
  assert.equal(orderClause(undefined), '');
  assert.equal(limitClause(10), 'LIMIT 10');
  assert.equal(limitClause(10, 20), 'LIMIT 10 OFFSET 20');
  assert.equal(limitClause(undefined), '');
  assert.throws(() => limitClause(-1), /whole number/);
  assert.equal(
    selectQuery('items', ['sku', 'title'], {
      filters: [{ column: 'sku', value: "A'1" }],
      order: { column: 'sku' },
      limit: 5,
    }),
    `SELECT "sku", "title" FROM "items" WHERE "sku" = 'A''1' ORDER BY "sku" ASC LIMIT 5;`,
  );
});

test('insert, bulk, update and delete statements quote their values', () => {
  assert.equal(
    insertQuery('items', { sku: "A'1", amount: 12050 }),
    `INSERT INTO "items" ("sku", "amount") VALUES ('A''1', 12050);`,
  );
  assert.equal(
    bulkInsertQuery('items', [{ sku: 'A', amount: 1 }, { sku: 'B', amount: 2 }]),
    `INSERT INTO "items" ("sku", "amount") VALUES ('A', 1), ('B', 2);`,
  );
  assert.equal(
    updateQuery('items', { title: "New'name", sku: 'A' }, 'sku', 'A'),
    `UPDATE "items" SET "title" = 'New''name' WHERE "sku" = 'A';`,
  );
  assert.equal(deleteQuery('items', 'sku', "A'1"), `DELETE FROM "items" WHERE "sku" = 'A''1';`);
  assert.throws(() => bulkInsertQuery('items', []), /non empty/);
  assert.throws(() => updateQuery('items', { sku: 'A' }, 'sku', 'A'), /nothing to update/);
});

test('the sql export writes identifiers and literals through the builder', () => {
  assert.equal(
    selectStatement('items', ['sku'], { filters: [{ column: 'category', value: "light'ing" }] }),
    `SELECT "sku" FROM "items" WHERE "category" = 'light''ing';`,
  );
  assert.equal(statementKind('select 1'), 'SELECT');
  assert.equal(statementKind('vacuum'), 'UNKNOWN');
  const exported = exportSelect(dataset(), { columns: ['sku', 'amount'], limit: 2 });
  assert.equal(exported, 'SELECT "sku", "amount" FROM "items" LIMIT 2;');
});

test('the sql export quotes an equal filter carried by the request', () => {
  const exported = exportFiltered(dataset(), [{ column: 'sku', value: "A'1" }], { columns: ['sku'] });
  assert.equal(exported, `SELECT "sku" FROM "items" WHERE "sku" = 'A''1';`);
});

test('insert exports cover every stored row', () => {
  const rows = exportInsert(dataset(), { columns: ['sku'] }).split('\n');
  assert.equal(rows.length, 2);
  assert.match(rows[0], /^INSERT INTO "items"/);
  assert.match(rows[1], /SKU-2/);
  const upsert = exportUpsert(dataset(), { sku: "A'1", title: 'Lamp', amount: 100 });
  assert.match(upsert, /ON CONFLICT \("sku"\) DO UPDATE SET/);
  assert.match(upsert, /"title" = 'Lamp'/);
  const search = exportSearch(dataset(), 'lamp', { column: 'title' });
  assert.match(search, /LIKE '%lamp%'/);
  assert.equal(statementsFor(dataset(), { includeInsert: true }).length, 2);
});

test('csv cells are quoted only when they must be', () => {
  assert.equal(escapeCell('plain'), 'plain');
  assert.equal(escapeCell('a,b'), '"a,b"');
  assert.equal(escapeCell('say "hi"'), '"say ""hi"""');
  assert.equal(escapeCell('a\nb'), '"a\nb"');
  assert.equal(escapeCell(undefined), '');
  assert.equal(escapeCell('a;b', ';'), '"a;b"');
});

test('csv writers keep the header and the row order', () => {
  const rows = [
    { sku: 'A1', title: 'Lamp, large' },
    { sku: 'B2', title: 'Say "hi"' },
  ];
  assert.equal(toCsv(rows, ['sku', 'title']), 'sku,title\nA1,"Lamp, large"\nB2,"Say ""hi"""\n');
  assert.equal(toTsv(rows, ['sku']), 'sku\nA1\nB2\n');
  assert.equal(toCsvByColumnTypes(rows, { columns: [{ name: 'sku' }, { name: 'title' }] }), toCsv(rows, ['sku', 'title']));
  assert.deepEqual(columnNames('sku,title\nA1,B'), ['sku', 'title']);
  assert.deepEqual(parseCsvLine('A1,"B,2"'), ['A1', 'B,2']);
  assert.equal(rowCount('a,b\n1,2\n3,4'), 2);
  assert.equal(withBom('a'), '\ufeffa');
  assert.match(fileNameFor('items'), /^items-\d{4}-\d{2}-\d{2}\.csv$/);
  assert.equal(csvChunks([1, 2, 3], ['n'], 2).length, 2);
});

test('the manifest records totals and detects changes', () => {
  const manifest = manifestFor(dataset(), { now: Date.UTC(2026, 1, 1) });
  assert.equal(manifest.dataset, 'items');
  assert.equal(manifest.rows, 2);
  assert.deepEqual(manifest.totals, { amount: 20950 });
  assert.equal(manifest.generated_at, '2026-02-01T00:00:00.000Z');
  assert.match(describeManifest(manifest), /items rows=2/);
  assert.match(describeManifest(manifest), /\$209\.50/);
  assert.equal(compareManifests(manifest, manifest).changed, false);
  const changed = compareManifests(manifest, { ...manifest, rows: 3 });
  assert.deepEqual(changed.fields, ['rows']);
  assert.equal(manifestLines(manifest).length, 5);
});
