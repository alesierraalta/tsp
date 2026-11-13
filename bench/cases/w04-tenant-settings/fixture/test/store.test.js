import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { MemoryStore } from '../src/store/memory.js';
import {
  applyFilters,
  clampLimit,
  countsBy,
  groupBy,
  paginate,
  projectFields,
  schemaFor,
  sortRows,
} from '../src/store/query.js';
import { SnapshotStore } from '../src/store/persist.js';
import { DocumentStore } from '../src/files/documents.js';
import { assertPlainName, extensionOf, mimeOf, resolveUnder } from '../src/files/paths.js';

function store() {
  return new MemoryStore({ clock: () => new Date('2026-01-01T00:00:00Z') });
}

test('insert stamps ids and refuses duplicates', () => {
  const db = store();
  const row = db.insert('tenants', { slug: 'acme', name: 'Acme', plan: 'team' });
  assert.match(row.id, /^ten_/);
  assert.equal(row.created_at, '2026-01-01T00:00:00.000Z');
  assert.throws(() => db.insert('tenants', { id: row.id }), /already exists/);
});

test('update mutates the stored row and remove deletes it', () => {
  const db = store();
  const row = db.insert('tenants', { name: 'Acme' });
  db.update('tenants', row.id, (current) => {
    current.name = 'Acme Two';
    return current;
  });
  assert.equal(db.get('tenants', row.id).name, 'Acme Two');
  assert.equal(db.update('tenants', row.id, () => undefined).id, row.id);
  db.remove('tenants', row.id);
  assert.equal(db.get('tenants', row.id), undefined);
  assert.throws(() => db.remove('tenants', row.id), /not found/);
});

test('find, first and names answer over a table', () => {
  const db = store();
  db.insert('tenants', { name: 'Acme', plan: 'team' });
  db.insert('tenants', { name: 'Globex', plan: 'team' });
  db.insert('tenants', { name: 'Initech', plan: 'free' });
  assert.equal(db.find('tenants', (row) => row.plan === 'team').length, 2);
  assert.equal(db.first('tenants', (row) => row.name === 'Initech').plan, 'free');
  assert.deepEqual(db.names(), ['tenants']);
  assert.equal(db.snapshot().tenants.length, 3);
});

test('transactions serialize', async () => {
  const db = store();
  const order = [];
  await Promise.all([
    db.transaction(async () => {
      order.push('first');
      await new Promise((resolve) => setTimeout(resolve, 5));
      order.push('first-done');
    }),
    db.transaction(async () => {
      order.push('second');
    }),
  ]);
  assert.deepEqual(order, ['first', 'first-done', 'second']);
});

test('filters understand equality and numeric bounds', () => {
  const rows = [
    { name: 'a', plan: 'team', retention_days: 10 },
    { name: 'b', plan: 'free', retention_days: 30 },
    { name: 'c', plan: 'team', retention_days: 90 },
  ];
  const schema = { fields: ['name', 'plan', 'retention_days'] };
  assert.equal(applyFilters(rows, { plan: 'team' }, schema).length, 2);
  assert.equal(applyFilters(rows, { retention_days_min: 30 }, schema).length, 2);
  assert.equal(applyFilters(rows, { retention_days_max: 10 }, schema).length, 1);
  assert.equal(applyFilters(rows, { unknown: 'x' }, schema).length, 3);
});

test('sorting uses the named field and direction', () => {
  const rows = [{ slug: 'b' }, { slug: 'a' }, { slug: 'c' }];
  const schema = { fields: ['slug'] };
  assert.deepEqual(sortRows(rows, { sort: 'slug' }, schema).map((row) => row.slug), ['a', 'b', 'c']);
  assert.deepEqual(sortRows(rows, { sort: 'slug', dir: 'desc' }, schema).map((row) => row.slug), ['c', 'b', 'a']);
  assert.throws(() => sortRows(rows, { sort: 'nope' }, schema), /cannot sort/);
});

test('pagination clamps the limit and reports the next offset', () => {
  const rows = Array.from({ length: 250 }, (_, index) => ({ id: index }));
  assert.equal(clampLimit('1000'), 100);
  assert.equal(clampLimit('0'), 25);
  assert.equal(clampLimit('nope', 10), 10);
  const page = paginate(rows, { limit: 100, offset: 100 });
  assert.equal(page.rows.length, 100);
  assert.equal(page.total, 250);
  assert.equal(page.next, '200');
  assert.equal(paginate(rows, { limit: 100, offset: 200 }).next, null);
});

test('projection keeps only known fields', () => {
  const schema = { fields: ['id', 'name'] };
  const rows = projectFields([{ id: 1, name: 'a', secret: 'x' }], ['id', 'nope'], schema);
  assert.deepEqual(rows, [{ id: 1 }]);
  assert.deepEqual(projectFields([{ id: 1, name: 'a', secret: 'x' }], [], schema), [
    { id: 1, name: 'a' },
  ]);
});

test('grouping and counting are stable', () => {
  const rows = [{ plan: 'team' }, { plan: 'free' }, { plan: 'team' }];
  assert.equal(groupBy(rows, 'plan').get('team').length, 2);
  assert.deepEqual({ ...countsBy(rows, 'plan') }, { team: 2, free: 1 });
  assert.deepEqual(Object.keys(schemaFor('tenants').fields).length > 0, true);
});

test('snapshot save and load round trip through the guarded path', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'w04-snap-'));
  try {
    const snapshots = new SnapshotStore({ dir, clock: () => new Date('2026-02-01T00:00:00Z') });
    const saved = await snapshots.save('tenants', [
      { id: 'ten_1', slug: 'acme', name: 'Acme', plan: 'team', settings: {} },
    ]);
    assert.equal(saved.rows, 1);
    const loaded = await snapshots.load('tenants');
    assert.equal(loaded.rows[0].slug, 'acme');
    assert.equal(loaded.written_at, '2026-02-01T00:00:00.000Z');
    const raw = JSON.parse(await readFile(join(dir, 'tenants.json'), 'utf8'));
    assert.equal(raw.collection, 'tenants');

    const db = store();
    await snapshots.restore(db, 'tenants');
    assert.equal(db.get('tenants', 'ten_1').plan, 'team');
    assert.deepEqual(await snapshots.names(), ['tenants']);
    assert.deepEqual(await snapshots.drop('tenants'), join(dir, 'tenants.json'));
    assert.deepEqual((await snapshots.load('tenants')).rows, []);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test('a snapshot file with broken json is reported', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'w04-snap-'));
  try {
    await writeFile(join(dir, 'tenants.json'), '{oops');
    const snapshots = new SnapshotStore({ dir });
    await assert.rejects(snapshots.load('tenants'), /not valid JSON/);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test('resolveUnder keeps names inside the root', () => {
  const root = '/srv/data';
  assert.equal(resolveUnder(root, 'notes.txt'), '/srv/data/notes.txt');
  assert.equal(resolveUnder(root, 'nested/notes.txt'), '/srv/data/nested/notes.txt');
  for (const name of ['../etc/passwd', '/etc/passwd', '..', 'a/../../x', '']) {
    assert.throws(() => resolveUnder(root, name), (error) => {
      assert.equal(error.code, 'unsafe_path');
      return true;
    });
  }
});

test('plain names reject separators and empty values', () => {
  assert.equal(assertPlainName('notes.txt'), 'notes.txt');
  for (const name of ['a/b', 'a\\b', '..', '', 'x'.repeat(200)]) {
    assert.throws(() => assertPlainName(name), /plain file name/);
  }
  assert.equal(extensionOf('report.csv'), 'csv');
  assert.equal(extensionOf('README'), '');
  assert.equal(mimeOf('report.csv').startsWith('text/csv'), true);
});

test('the document store reads, lists and writes inside its root', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'w04-docs-'));
  try {
    const documents = new DocumentStore({ root: dir });
    await documents.write('notes.txt', 'hello');
    await documents.append('notes.txt', ' world');
    assert.equal(await documents.readText('notes.txt'), 'hello world');
    assert.deepEqual((await documents.list()).map((entry) => entry.name), ['notes.txt']);
    const described = await documents.describe('notes.txt');
    assert.equal(described.type, 'text/plain; charset=utf-8');
    assert.equal((await documents.usage()).files, 1);
    await documents.remove('notes.txt');
    await assert.rejects(documents.read('notes.txt'), /not found/);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test('the document store refuses a name that leaves its root', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'w04-docs-'));
  try {
    const documents = new DocumentStore({ root: join(dir, 'public') });
    await assert.rejects(documents.read('../private.txt'), (error) => {
      assert.equal(error.code, 'unsafe_path');
      return true;
    });
    await assert.rejects(documents.write('../escape.txt', 'x'), /plain file name/);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
