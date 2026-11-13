import test from 'node:test';
import assert from 'node:assert/strict';
import { MemoryStore } from '../src/store/memory.js';
import { PartitionedStore, partitionReport, partitionWindow } from '../src/store/partitions.js';
import { SnapshotBook, snapshotSummary } from '../src/store/snapshots.js';
import { AuditLog, auditSummary, auditWindow, auditActors } from '../src/store/audit.js';
import { SeriesStore, rollupPoints } from '../src/store/series.js';
import { EventIndexes, countDistinct, daysCovered, hottestName, userDayMatrix } from '../src/store/indexes.js';
import { queryRows, matches, readPath, groupCounts, project, projectRows, explainQuery, aggregateBy, windowRows, distinctValues, countRows, describeQuery } from '../src/store/query.js';
import { clock } from './helpers.js';

const event = (id, at, extra = {}) => ({
  id,
  tenant: 'acme',
  name: 'page_view',
  at,
  context: { user: 'usr_1', path: '/docs' },
  ...extra,
});

test('the memory store keys rows by id and refuses a second insert', () => {
  const store = new MemoryStore({ clock });
  store.insert('events', 'evt_1', event('evt_1', '2026-02-09T10:00:00.000Z'));
  assert.equal(store.get('events', 'evt_1').name, 'page_view');
  assert.throws(() => store.insert('events', 'evt_1', event('evt_1', '2026-02-09T10:00:00.000Z')), /already exists/);
  store.update('events', 'evt_1', { name: 'click' });
  assert.equal(store.get('events', 'evt_1').name, 'click');
  assert.equal(store.find('events', (row) => row.name === 'click').length, 1);
  assert.equal(store.remove('events', 'evt_1'), true);
  assert.equal(store.remove('events', 'evt_1'), false);
  assert.equal(store.stats().writes >= 3, true);
  const copy = store.get('events', 'evt_1');
  assert.equal(copy, undefined);
});

test('a partition holds one day of one tenant and can be compacted or swept', () => {
  const partitions = new PartitionedStore({ clock });
  partitions.addAll('acme', [event('evt_1', '2026-02-08T10:00:00.000Z'), event('evt_2', '2026-02-09T10:00:00.000Z')]);
  partitions.add('globex', event('evt_3', '2026-02-09T11:00:00.000Z', { tenant: 'globex' }));
  assert.deepEqual(partitions.days('acme'), ['2026-02-08', '2026-02-09']);
  assert.equal(partitions.partition('acme', '2026-02-09').length, 1);
  assert.equal(partitions.between('acme', '2026-02-09', '2026-02-09').length, 1);
  assert.deepEqual(partitions.daySizes('acme'), [{ day: '2026-02-08', rows: 1 }, { day: '2026-02-09', rows: 1 }]);
  assert.equal(partitionWindow(partitions, 'acme').from, '2026-02-08');
  assert.equal(partitionReport(partitions).rows, 3);
  const planned = partitions.sweep('acme', '2026-02-09', { dry_run: true });
  assert.deepEqual(planned.days, ['2026-02-08']);
  assert.equal(planned.dropped, 0);
  assert.equal(partitions.days('acme').length, 2);
  assert.equal(partitions.sweep('acme', '2026-02-09').dropped, 1);
  assert.deepEqual(partitions.days('acme'), ['2026-02-09']);
  assert.equal(partitions.compactAll('acme').days, 1);
});

test('compaction folds repeated ids into the last row written', () => {
  const partitions = new PartitionedStore({ clock });
  partitions.partitions.set('acme|2026-02-09', [
    event('evt_1', '2026-02-09T10:00:00.000Z', { name: 'page_view' }),
    event('evt_1', '2026-02-09T11:00:00.000Z', { name: 'click' }),
    event('evt_2', '2026-02-09T09:00:00.000Z'),
  ]);
  const result = partitions.compact('acme', '2026-02-09');
  assert.deepEqual(result, { rows: 3, folded: 1, kept: 2 });
  assert.deepEqual(partitions.partition('acme', '2026-02-09').map((row) => row.id), ['evt_2', 'evt_1']);
});

test('a snapshot can be restored, diffed and pruned by instant', () => {
  const book = new SnapshotBook({ clock, limit: 3 });
  book.take('first', { events: 1 });
  book.take('second', { events: 2, users: 1 });
  assert.deepEqual(book.restore('second'), { events: 2, users: 1 });
  assert.deepEqual(book.diff('first', 'second'), ['events', 'users']);
  assert.deepEqual(book.diffReport('first', 'second').added, ['users']);
  assert.deepEqual(book.list().map((snapshot) => snapshot.name), ['first', 'second']);
  assert.equal(snapshotSummary(book).newest.name, 'second');
  assert.equal(book.pruneBefore('2026-02-11T00:00:00.000Z'), 2);
  assert.equal(book.size(), 0);
  assert.equal(book.stats().limit, 3);
});

test('the audit log records the actions it knows and prunes by instant', () => {
  const audit = new AuditLog({ clock, limit: 5 });
  audit.record('key.issued', { tenant: 'acme', actor: 'admin_1', target: 'key_1' });
  audit.record('plan.changed', { tenant: 'acme', actor: 'admin_1', target: 'growth' });
  audit.record('retention.swept', { tenant: 'globex', actor: 'worker', target: 'free', details: { removed: 3 } });
  assert.throws(() => audit.record('nope.happened'), /unknown audit action/);
  assert.equal(audit.size(), 3);
  assert.deepEqual(audit.counts(), { 'key.issued': 1, 'plan.changed': 1, 'retention.swept': 1 });
  assert.equal(audit.list({ tenant: 'acme' }).length, 2);
  assert.equal(audit.list({ action: 'plan.changed' })[0].target, 'growth');
  assert.deepEqual(auditActors(audit, 'acme'), [{ actor: 'admin_1', rows: 2 }]);
  assert.equal(audit.exportRows({ action: 'plan.changed' })[0].actor, 'admin_1');
  assert.equal(auditSummary(audit).rows, 3);
  assert.equal(auditWindow(audit, '2026-02-10T00:00:00.000Z', '2026-02-10T23:00:00.000Z', 'acme').rows, 2);
  assert.equal(audit.exportRows({ tenant: 'acme' }).length, 2);
  assert.equal(audit.prune('2026-02-11T00:00:00.000Z'), 3);
  assert.equal(audit.size(), 0);
});

test('a series keeps hourly points and compacts them to days', () => {
  const series = new SeriesStore({ clock });
  series.add('acme', 'page_view', '2026-02-09T10:10:00.000Z', 2);
  series.add('acme', 'page_view', '2026-02-09T10:20:00.000Z', 3);
  series.add('acme', 'page_view', '2026-02-09T11:00:00.000Z', 1);
  const points = series.points('acme', 'page_view');
  assert.equal(points.length, 2);
  assert.equal(points[0].count, 2);
  assert.equal(series.total('acme', 'page_view'), 6);
  assert.equal(series.between('acme', 'page_view', '2026-02-09T10:00:00.000Z', '2026-02-09T10:59:00.000Z').length, 1);
  assert.deepEqual(series.dimensions('acme', 'page_view'), []);
  const compacted = series.compact('acme', 'page_view');
  assert.deepEqual(compacted, { before: 2, after: 1 });
  assert.equal(series.points('acme', 'page_view')[0].total, 6);
  assert.equal(rollupPoints([{ bucket: 'a', count: 1, total: 2 }], 1)[0].total, 2);
  assert.equal(series.prune('2026-02-10T00:00:00.000Z'), 1);
  assert.equal(series.stats().series, 0);
});

test('the event indexes answer by name, user and day', () => {
  const rows = [event('evt_1', '2026-02-09T10:00:00.000Z'), event('evt_2', '2026-02-09T11:00:00.000Z', { context: { user: 'usr_2', path: '/a' } })];
  const indexes = new EventIndexes(rows);
  assert.deepEqual(daysCovered(indexes), ['2026-02-09']);
  assert.equal(indexes.names('page_view').length, 2);
  assert.equal(indexes.user('usr_2').length, 1);
  assert.equal(hottestName(indexes).name, 'page_view');
  assert.equal(countDistinct(rows, 'context.user'), 2);
  assert.deepEqual(userDayMatrix(indexes, ['usr_2']), [{ user: 'usr_2', days: 1, total: 1 }]);
  indexes.remove(rows[0]);
  assert.equal(indexes.names('page_view').length, 1);
});

test('a query filters, sorts, pages and explains what it matched', () => {
  const rows = [
    { id: 'a', name: 'page_view', at: '2026-02-09T10:00:00.000Z', context: { user: 'usr_1' } },
    { id: 'b', name: 'purchase', at: '2026-02-09T12:00:00.000Z', context: { user: 'usr_2' } },
    { id: 'c', name: 'page_view', at: '2026-02-10T09:00:00.000Z', context: { user: 'usr_1' } },
  ];
  const result = queryRows(rows, { where: [{ path: 'name', value: 'page_view' }], sort: '-at' });
  assert.deepEqual(result.rows.map((entry) => entry.id), ['c', 'a']);
  assert.equal(matches(rows[0], [{ path: 'context.user', op: 'eq', value: 'usr_1' }]), true);
  assert.equal(readPath(rows[1], 'context.user'), 'usr_2');
  assert.deepEqual(project(rows[0], ['name', 'context.user']), { name: 'page_view', context_user: 'usr_1' });
  assert.equal(projectRows(rows, ['id']).length, 3);
  assert.equal(countRows(rows, [{ path: 'name', value: 'page_view' }]), 2);
  assert.equal(distinctValues(rows, 'context.user').length, 2);
  assert.equal(windowRows(rows, '2026-02-09T00:00:00.000Z', '2026-02-09T23:59:59.000Z').length, 2);
  assert.deepEqual(groupCounts(rows, 'name')[0], { value: 'page_view', count: 2 });
  assert.deepEqual(aggregateBy(rows, 'name', 'id', 'count')[0], { key: 'page_view', value: 2, rows: 2 });
  const explained = explainQuery(rows, { where: [{ path: 'name', value: 'page_view' }], limit: 1 });
  assert.deepEqual(explained.matched, 2);
  assert.equal(explained.returned, 1);
  assert.equal(explained.has_more, true);
  assert.deepEqual(describeQuery({ where: [{ path: 'name', value: 'page_view' }], sort: '-at', limit: 1 }).sort, ['-at']);
});
