import test from 'node:test';
import assert from 'node:assert/strict';
import { MemoryStore } from '../src/store/memory.js';
import { EventLog, eventSummary, topicRows, actorReport } from '../src/store/events.js';
import { Transaction, withTransaction } from '../src/store/transactions.js';
import { StoreSet, describeCollections, totalRows } from '../src/store/index.js';
import { Cache, cachedDashboard, invalidateTenant } from '../src/store/cache.js';
import { readPath, matches, compareValues, sortRows, parseSort, queryRows, aggregate, groupCounts, project, distinctValues, explainQuery, windowRows, selectColumns } from '../src/store/query.js';
import { clock } from './helpers.js';

const release = (id, extra = {}) => ({ id, tenant: 'acme', service: 'checkout', status: 'planned', created_at: '2026-02-09T10:00:00.000Z', ...extra });

test('the memory store keys rows by id and refuses a second insert', () => {
  const store = new MemoryStore({ clock });
  store.insert('releases', 'rel_1', release('rel_1'));
  assert.equal(store.get('releases', 'rel_1').service, 'checkout');
  assert.throws(() => store.insert('releases', 'rel_1', release('rel_1')), /already exists/);
  store.update('releases', 'rel_1', { status: 'staged' });
  assert.equal(store.get('releases', 'rel_1').status, 'staged');
  assert.equal(store.find('releases', (row) => row.status === 'staged').length, 1);
  assert.equal(store.has('releases', 'rel_1'), true);
  assert.deepEqual(store.ids('releases'), ['rel_1']);
  assert.equal(store.remove('releases', 'rel_1'), true);
  assert.equal(store.remove('releases', 'rel_1'), false);
  assert.throws(() => store.require('releases', 'rel_1'), /not found/);
  assert.equal(store.stats().writes >= 3, true);
});

test('the event log records topics with actor and subject', () => {
  const log = new EventLog({ clock, limit: 3 });
  log.append('release.created', { version: '1.0.0' }, { tenant: 'acme', actor: 'usr_1', subject: 'rel_1' });
  log.append('deployment.started', {}, { tenant: 'acme', actor: 'usr_2', subject: 'dep_1' });
  assert.throws(() => log.append('nope.happened'), /unknown event topic/);
  assert.equal(log.size(), 2);
  assert.equal(log.list({ topic: 'release.created' }).length, 1);
  assert.equal(log.list({ subject: 'dep_1' })[0].topic, 'deployment.started');
  assert.deepEqual(log.counts(), { 'release.created': 1, 'deployment.started': 1 });
  assert.deepEqual(eventSummary(log).busiest[1], 1);
  assert.equal(topicRows(log, 'acme').length, 2);
  assert.deepEqual(actorReport(log)[0].actor !== undefined, true);
  assert.equal(log.since('2026-02-10T00:00:00.000Z').length, 2);
  log.append('approval.granted', {}, { tenant: 'acme' });
  log.append('incident.opened', {}, { tenant: 'acme' });
  assert.equal(log.size(), 3);
  assert.equal(log.clear(), 3);
  assert.equal(log.stats().events, 0);
});

test('a transaction commits its pending writes and drops them on rollback', () => {
  const store = new MemoryStore({ clock });
  store.insert('releases', 'rel_1', release('rel_1'));
  const transaction = new Transaction(store, { clock });
  transaction.write('releases', 'rel_2', release('rel_2'));
  transaction.remove('releases', 'rel_1');
  assert.equal(transaction.read('releases', 'rel_2').service, 'checkout');
  assert.equal(transaction.all('releases').length, 1);
  assert.equal(store.count('releases'), 1);
  assert.deepEqual(transaction.commit(), { written: 1, removed: 1, reads: 1 });
  assert.equal(store.get('releases', 'rel_1'), undefined);
  assert.equal(store.get('releases', 'rel_2').id, 'rel_2');
  assert.throws(() => transaction.commit(), /transaction is committed/);
});

test('a rolled back transaction leaves the store untouched', () => {
  const store = new MemoryStore({ clock });
  store.insert('releases', 'rel_1', release('rel_1'));
  const transaction = new Transaction(store, { clock });
  transaction.write('releases', 'rel_2', release('rel_2'));
  assert.deepEqual(transaction.rollback(), { discarded: 1 });
  assert.equal(store.count('releases'), 1);
  assert.deepEqual(transaction.stats().state, 'rolled_back');
  assert.throws(
    () =>
      withTransaction(store, (tx) => {
        tx.write('releases', 'rel_3', release('rel_3'));
        throw new Error('boom');
      }),
    /boom/,
  );
  assert.equal(store.count('releases'), 1);
});

test('the store set reports its collections', () => {
  const set = new StoreSet({ clock });
  set.store.insert('releases', 'rel_1', release('rel_1'));
  set.events.append('release.created', {}, { tenant: 'acme' });
  assert.deepEqual(set.collectionStats().collections, { releases: 1 });
  assert.equal(set.report().events.events, 1);
  assert.equal(describeCollections(set.store)[0].collection, 'releases');
  assert.equal(totalRows(set.store), 1);
  assert.deepEqual(set.reset().collections, { releases: 1 });
  assert.equal(set.store.count('releases'), 0);
});

test('a cached value is returned until it expires', () => {
  let now = new Date('2026-02-10T12:00:00.000Z');
  const cache = new Cache({ clock: () => now, ttlMs: 60000, limit: 3 });
  let calls = 0;
  const value = cachedDashboard(cache, 'acme', 'staging', () => {
    calls += 1;
    return { releases: 2 };
  });
  assert.deepEqual(value, { releases: 2 });
  cachedDashboard(cache, 'acme', 'staging', () => {
    calls += 1;
    return { releases: 3 };
  });
  assert.equal(calls, 1);
  now = new Date('2026-02-10T12:02:00.000Z');
  assert.equal(cache.get('dashboard', { tenant: 'acme', environment: 'staging' }), undefined);
  assert.equal(cache.stats().misses, 2);
  cache.set('dashboard', { tenant: 'acme', environment: 'production' }, { releases: 1 });
  assert.equal(invalidateTenant(cache, 'acme'), 2);
  assert.equal(cache.stats().entries, 0);
});

test('a query filters, sorts, pages and explains what it matched', () => {
  const rows = [
    { id: 'a', service: 'checkout', status: 'planned', created_at: '2026-02-09T10:00:00.000Z', meta: { owner: 'usr_1' } },
    { id: 'b', service: 'checkout', status: 'live', created_at: '2026-02-09T12:00:00.000Z', meta: { owner: 'usr_2' } },
    { id: 'c', service: 'search', status: 'planned', created_at: '2026-02-10T09:00:00.000Z' },
  ];
  const result = queryRows(rows, { where: [{ path: 'service', value: 'checkout' }], sort: '-created_at' });
  assert.deepEqual(result.rows.map((row) => row.id), ['b', 'a']);
  assert.equal(matches(rows[0], [{ path: 'meta.owner', op: 'eq', value: 'usr_1' }]), true);
  assert.equal(compareValues(1, 2) < 0, true);
  assert.deepEqual(parseSort('-created_at'), [{ path: 'created_at', direction: 'desc' }]);
  assert.deepEqual(sortRows(rows, parseSort('service')).map((row) => row.id), ['a', 'b', 'c']);
  assert.equal(readPath(rows[0], 'meta.owner'), 'usr_1');
  assert.equal(aggregate(rows, 'status', 'distinct'), 2);
  assert.equal(aggregate(rows, 'id', 'count'), 3);
  assert.deepEqual(groupCounts(rows, 'service')[0], { value: 'checkout', count: 2 });
  assert.deepEqual(project(rows[0], ['service', 'meta.owner']), { service: 'checkout', meta_owner: 'usr_1' });
  assert.deepEqual(selectColumns(rows, ['id'])[0], { id: 'a' });
  assert.equal(distinctValues(rows, 'status').length, 2);
  assert.equal(windowRows(rows, '2026-02-09T00:00:00.000Z', '2026-02-09T23:59:59.000Z', 'created_at').length, 2);
  const explained = explainQuery(rows, { where: [{ path: 'status', value: 'planned' }], limit: 1 });
  assert.deepEqual(explained.matched, 2);
  assert.equal(explained.returned, 1);
  assert.equal(explained.has_more, true);
});
