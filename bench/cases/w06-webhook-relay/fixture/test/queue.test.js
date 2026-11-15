import test from 'node:test';
import assert from 'node:assert/strict';
import { DeliveryStore, STATUSES } from '../src/queue/store.js';
import { OffsetStore, nextBatch } from '../src/queue/offsets.js';
import {
  canRetry,
  classify,
  describeBackoff,
  exhaustedError,
  nextAttemptAt,
  nextDelay,
  scheduleRetry,
} from '../src/jobs/retry.js';
import { loadConfig } from '../src/config.js';

function store(clock = () => new Date('2026-01-01T00:00:00Z')) {
  return new DeliveryStore({ clock });
}

function delivery(overrides = {}) {
  return {
    event: 'order.created',
    topic: 'orders',
    url: 'https://hooks.example.com/a',
    entity_id: 'ord_1',
    payload: { id: 'ord_1' },
    ...overrides,
  };
}

test('the store stamps and keeps a delivery', () => {
  const db = store();
  const row = db.insert(delivery());
  assert.match(row.id, /^dlv_/);
  assert.equal(row.status, 'pending');
  assert.equal(row.attempts, 0);
  assert.equal(row.created_at, '2026-01-01T00:00:00.000Z');
  assert.equal(db.get(row.id).url, 'https://hooks.example.com/a');
  assert.equal(db.count(), 1);
  assert.deepEqual(db.counts(), { total: 1, pending: 1, retrying: 0, delivered: 0, failed: 0 });
  assert.equal(STATUSES.length, 4);
});

test('an equivalent queued delivery is refused', () => {
  const db = store();
  db.insert(delivery());
  assert.throws(() => db.insert(delivery()), /equivalent delivery/);
  const other = db.insert(delivery({ entity_id: 'ord_2' }));
  assert.ok(other.id);
  assert.throws(() => db.insert({ ...delivery({ entity_id: 'ord_3' }), id: other.id }), /already exists/);
});

test('lookups and updates report what they cannot find', () => {
  const db = store();
  const row = db.insert(delivery());
  db.update(row.id, { status: 'retrying' });
  assert.equal(db.get(row.id).status, 'retrying');
  assert.throws(() => db.get('dlv_none'), /not found/);
  assert.throws(() => db.update('dlv_none', {}), /not found/);
  assert.throws(() => db.remove('dlv_none'), /not found/);
  assert.equal(db.remove(row.id).id, row.id);
  assert.equal(db.count(), 0);
});

test('list filters by status and topic', () => {
  const db = store();
  db.insert(delivery({ entity_id: 'a', topic: 'orders' }));
  db.insert(delivery({ entity_id: 'b', topic: 'items' }));
  const pending = db.insert(delivery({ entity_id: 'c', topic: 'orders' }));
  db.markDelivered(pending.id, { status: 200 });
  assert.equal(db.list().length, 3);
  assert.equal(db.list({ topic: 'items' }).length, 1);
  assert.equal(db.list({ status: 'delivered' }).length, 1);
  assert.equal(db.list({ limit: 1 }).length, 1);
});

test('due returns pending rows whose time has come', () => {
  let now = '2026-01-01T00:00:00.000Z';
  const db = store(() => new Date(now));
  const early = db.insert(delivery({ entity_id: 'early', next_attempt_at: '2026-01-01T00:00:00.000Z' }));
  const later = db.insert(delivery({ entity_id: 'later', next_attempt_at: '2026-01-02T00:00:00.000Z' }));
  assert.deepEqual(db.due(now).map((row) => row.id), [early.id]);
  assert.deepEqual(db.due('2026-01-03T00:00:00Z').map((row) => row.id).sort(), [early.id, later.id].sort());
});

test('marking attempts and outcomes moves the row', () => {
  const db = store();
  const row = db.insert(delivery());
  db.markAttempt(row.id, new Error('boom'));
  assert.equal(db.get(row.id).attempts, 1);
  assert.equal(db.get(row.id).last_error.message, 'boom');
  db.markRetrying(row.id, '2026-01-01T00:01:00.000Z', { message: 'again', code: 'delivery_failed' });
  assert.equal(db.get(row.id).status, 'retrying');
  assert.equal(db.get(row.id).next_attempt_at, '2026-01-01T00:01:00.000Z');
  db.markDelivered(row.id, { status: 200 });
  assert.equal(db.get(row.id).status, 'delivered');
  assert.equal(db.get(row.id).last_error, null);
  const second = db.insert(delivery({ entity_id: 'two' }));
  db.markFailed(second.id, new Error('dead'));
  assert.equal(db.get(second.id).status, 'failed');
  db.clear();
  assert.equal(db.count(), 0);
});

test('offsets only move forward', () => {
  const offsets = new OffsetStore();
  assert.equal(offsets.read('orders'), 0);
  assert.equal(offsets.advance('orders', 'worker', 5), 5);
  assert.equal(offsets.advance('orders', 'worker', 3), 5);
  assert.equal(offsets.read('orders', 'worker'), 5);
  offsets.advance('items', 'worker', 2);
  assert.deepEqual(offsets.topics(), ['items', 'orders']);
  assert.deepEqual(offsets.consumers('orders'), ['worker']);
  assert.deepEqual(offsets.snapshot(), { 'orders:worker': 5, 'items:worker': 2 });
  assert.equal(offsets.reset('orders', 'worker'), 5);
  assert.equal(offsets.read('orders', 'worker'), 0);
  assert.throws(() => offsets.advance('orders', 'worker', -1), /whole number/);
  assert.deepEqual(nextBatch([1, 2, 3, 4], 1, 2), [2, 3]);
});

test('backoff doubles and caps', () => {
  assert.equal(nextDelay(1), 250);
  assert.equal(nextDelay(2), 500);
  assert.equal(nextDelay(3), 1000);
  assert.equal(nextDelay(30), 30_000);
  assert.equal(nextDelay(1, { baseMs: 10, maxMs: 25 }), 10);
  assert.equal(nextAttemptAt(1, '2026-01-01T00:00:00Z'), '2026-01-01T00:00:00.250Z');
  assert.deepEqual(describeBackoff({ maxAttempts: 3 }), [250, 500, 1000]);
  assert.throws(() => nextAttemptAt(1, 'nope'), /not a date/);
});

test('a delivery is retried until maxAttempts', () => {
  const db = store();
  const config = loadConfig({ maxAttempts: 2 });
  const row = db.insert(delivery());
  assert.equal(canRetry(row, config), true);
  db.markAttempt(row.id);
  const first = scheduleRetry(db, row.id, { message: 'x', code: 'delivery_failed' }, config);
  assert.equal(first.scheduled, true);
  db.markAttempt(row.id);
  const second = scheduleRetry(db, row.id, { message: 'x', code: 'delivery_failed' }, config);
  assert.equal(second.scheduled, false);
  assert.equal(db.get(row.id).status, 'failed');
  assert.match(exhaustedError(db.get(row.id)).message, /after 2 attempts/);
});

test('error classification decides retryability', () => {
  assert.deepEqual(classify(new Error('x')), { retryable: true, reason: 'unexpected' });
  assert.deepEqual(classify({ code: 'delivery_failed' }), { retryable: true, reason: 'delivery_failed' });
  assert.deepEqual(classify({ code: 'egress_blocked', retryable: false }), {
    retryable: false,
    reason: 'egress_blocked',
  });
  assert.equal(classify(undefined).retryable, true);
});
