import test from 'node:test';
import assert from 'node:assert/strict';
import { MemoryStore } from '../src/store/memory.js';
import { applyQuery, aggregate, groupAggregate, matchesAll, parseSort, sortRows, compareValues, encodeCursor, decodeCursor, readPath } from '../src/store/query.js';
import { buildIndexes, SecondaryIndex, IndexSet } from '../src/store/index.js';
import { RevisionLog } from '../src/store/revisions.js';
import { runInTransaction, runSync, WriteQueue } from '../src/store/transactions.js';

function seeded() {
  const store = new MemoryStore({ clock: () => new Date('2026-03-05T10:00:00.000Z') });
  store.insert('entries', 'e1', { id: 'e1', slug: 'alpha', status: 'published', values: { title: 'Alpha', views: 10 } });
  store.insert('entries', 'e2', { id: 'e2', slug: 'beta', status: 'draft', values: { title: 'Beta', views: 4 } });
  store.insert('entries', 'e3', { id: 'e3', slug: 'gamma', status: 'published', values: { title: 'Gamma', views: 7 } });
  return store;
}

test('the memory store clones on the way in and out', () => {
  const store = seeded();
  const row = store.get('entries', 'e1');
  row.values.title = 'changed';
  assert.equal(store.get('entries', 'e1').values.title, 'Alpha');
  assert.equal(store.count('entries'), 3);
  assert.equal(store.writes, 3);
});

test('the memory store updates, removes and restores snapshots', () => {
  const store = seeded();
  store.update('entries', 'e2', { status: 'published' });
  assert.equal(store.get('entries', 'e2').status, 'published');
  const snapshot = store.snapshot();
  store.remove('entries', 'e1');
  assert.equal(store.has('entries', 'e1'), false);
  store.restore(snapshot);
  assert.equal(store.has('entries', 'e1'), true);
  assert.equal(store.reduce('entries', (total, row) => total + row.values.views, 0), 21);
  assert.equal(store.latest('entries', 'id').id, 'e3');
});

test('queries filter, sort and page', () => {
  const store = seeded();
  const page = applyQuery(store.all('entries'), { where: [{ path: 'status', op: 'eq', value: 'published' }], sort: [{ path: 'values.views', direction: 'desc' }], limit: 1 });
  assert.equal(page.total, 2);
  assert.equal(page.rows[0].slug, 'alpha');
  assert.equal(page.has_more, true);
  assert.equal(matchesAll({ a: 1 }, [{ path: 'a', op: 'gte', value: 1 }]), true);
  assert.deepEqual(parseSort('-slug'), [{ path: 'slug', direction: 'desc' }]);
  assert.equal(sortRows([{ n: 2 }, { n: 1 }], [{ path: 'n' }])[0].n, 1);
  assert.equal(compareValues('a', 'b'), -1);
});

test('aggregates group by a path', () => {
  const store = seeded();
  assert.equal(aggregate(store.all('entries'), 'values.views', 'sum'), 21);
  assert.equal(aggregate(store.all('entries'), 'values.views', 'max'), 10);
  assert.deepEqual(groupAggregate(store.all('entries'), 'status'), { published: 2, draft: 1 });
});

test('cursors round-trip and skip rows', () => {
  const cursor = encodeCursor({ updated_at: '2026-03-05T10:00:00.000Z', id: 'e2' });
  const decoded = decodeCursor(cursor);
  assert.equal(decoded.id, 'e2');
  assert.equal(decodeCursor('not a cursor'), undefined);
  assert.equal(readPath({ a: { b: 1 } }, 'a.b'), 1);
});

test('secondary indexes find rows by value', () => {
  const rows = [
    { id: 'e1', status: 'published', tags: ['news', 'guide'] },
    { id: 'e2', status: 'draft', tags: ['news'] },
  ];
  const set = buildIndexes(rows, { status: { path: 'status' }, tags: { path: 'tags', multi: true } });
  assert.deepEqual(set.get('status').lookup('published'), ['e1']);
  assert.deepEqual(set.get('tags').lookup('news').sort(), ['e1', 'e2']);
  set.remove(rows[0]);
  assert.deepEqual(set.get('status').lookup('published'), []);
  const index = new SecondaryIndex('status', 'status');
  for (const row of rows) index.add(row);
  assert.equal(index.size(), 2);
  const other = new IndexSet(rows);
  assert.deepEqual(other.stats(), {});
});

test('the revision log keeps a bounded history', () => {
  const log = new RevisionLog({ clock: () => new Date('2026-03-05T10:00:00.000Z'), limit: 2 });
  log.record({ collection: 'entries', id: 'e1', action: 'create', after: { title: 'a' }, actor: 'sam' });
  log.record({ collection: 'entries', id: 'e1', action: 'update', before: { title: 'a' }, after: { title: 'b' }, actor: 'sam' });
  log.record({ collection: 'entries', id: 'e1', action: 'update', before: { title: 'b' }, after: { title: 'c' }, actor: 'kim' });
  assert.equal(log.revisionsOf('entries', 'e1').length, 2);
  assert.deepEqual(log.restore('entries', 'e1', 3), { title: 'c' });
  assert.equal(log.actions().update, 2);
  assert.equal(log.byActor('sam').length, 1);
  assert.deepEqual(log.compare('entries', 'e1', 2, 3), { title: { before: 'b', after: 'c' } });
});

test('transactions roll back on failure', async () => {
  const store = seeded();
  await assert.rejects(
    runInTransaction(store, async (transaction) => {
      store.insert('entries', 'e4', { id: 'e4' });
      assert.equal(transaction.finished, false);
      throw new Error('boom');
    }),
    /boom/,
  );
  assert.equal(store.has('entries', 'e4'), false);
  const value = runSync(store, () => {
    store.insert('entries', 'e5', { id: 'e5' });
    return 'done';
  });
  assert.equal(value, 'done');
  assert.equal(store.has('entries', 'e5'), true);
  const queue = new WriteQueue();
  store.count('entries');
  assert.equal(await queue.push(async () => 1), 1);
});
