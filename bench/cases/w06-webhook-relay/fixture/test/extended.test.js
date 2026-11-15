import test from 'node:test';
import assert from 'node:assert/strict';
import {
  businessWindow,
  describeWindow,
  formatClock,
  inQuietHours,
  isOpen,
  isWeekend,
  minuteOfDay,
  minutesUntilOpen,
  nextOpen,
  parseClock,
  parseQuietHours,
  parseWindow,
  weekdayOf,
} from '../src/queue/window.js';
import {
  DEFAULT_TOPICS,
  TopicRegistry,
  fanoutPlan,
  topicSummary,
  validateTopicName,
} from '../src/jobs/topics.js';
import {
  allowlistFromEnv,
  describeAllowlist,
  diffAllowlist,
  isAllowlisted,
  matchesEntry,
  parseAllowlist,
  validateEntry,
} from '../src/egress/allowlist.js';
import {
  DEFAULT_ROTATION,
  ageDays,
  ageOf,
  planRotation,
  rotationDue,
  rotationSummary,
  usableKeys,
  withinGrace,
} from '../src/auth/rotation.js';
import { AuditLog, auditSummary } from '../src/observability/audit.js';
import {
  assertPayloadSize,
  digestOf,
  envelope,
  envelopeSummary,
  parseEnvelope,
  payloadBytes,
  redactEnvelope,
  sameEnvelope,
} from '../src/webhook/payload.js';

test('clocks and windows parse both shapes', () => {
  assert.equal(parseClock('09:30'), 570);
  assert.equal(formatClock(570), '09:30');
  assert.equal(formatClock(1440), '00:00');
  assert.throws(() => parseClock('24:00'), /clock time/);
  assert.throws(() => parseClock('9:00'), /clock time/);
  const day = parseWindow('09:00-17:00');
  assert.equal(day.minutes, 480);
  assert.equal(day.overnight, false);
  assert.equal(describeWindow(day), '09:00-17:00');
  const night = parseWindow('22:00-06:00');
  assert.equal(night.overnight, true);
  assert.equal(night.minutes, 480);
  assert.throws(() => parseWindow('09:00'), /not a window/);
});

test('isOpen understands daytime and overnight windows', () => {
  const day = parseWindow('09:00-17:00');
  assert.equal(isOpen(day, new Date('2026-01-05T10:00:00')), true);
  assert.equal(isOpen(day, new Date('2026-01-05T20:00:00')), false);
  assert.equal(minuteOfDay(new Date('2026-01-05T00:30:00')), 30);
  assert.equal(minutesUntilOpen(day, new Date('2026-01-05T08:00:00')), 60);
  assert.equal(minutesUntilOpen(day, new Date('2026-01-05T10:00:00')), 0);
  const night = parseWindow('22:00-06:00');
  assert.equal(isOpen(night, new Date('2026-01-05T23:00:00')), true);
  assert.equal(isOpen(night, new Date('2026-01-05T05:00:00')), true);
  assert.equal(isOpen(night, new Date('2026-01-05T12:00:00')), false);
  assert.ok(new Date(nextOpen(day, new Date('2026-01-05T08:00:00'))).getTime() > 0);
});

test('quiet hours and business hours compose', () => {
  const quiet = parseQuietHours('22:00-06:00, 12:00-13:00');
  assert.equal(quiet.length, 2);
  assert.equal(inQuietHours(quiet, new Date('2026-01-05T12:30:00')), true);
  assert.equal(inQuietHours(quiet, new Date('2026-01-05T15:00:00')), false);
  assert.deepEqual(parseQuietHours(''), []);
  assert.equal(describeWindow(businessWindow()), '09:00-17:00');
  assert.equal(weekdayOf(new Date('2026-01-05T00:00:00')), 'mon');
  assert.equal(isWeekend(new Date('2026-01-04T00:00:00')), true);
  assert.equal(isWeekend(new Date('2026-01-05T00:00:00')), false);
});

test('topics validate, route and describe', () => {
  assert.equal(validateTopicName(' Orders '), 'orders');
  assert.throws(() => validateTopicName('nope topic'), /not a topic name/);
  const registry = new TopicRegistry(DEFAULT_TOPICS);
  assert.equal(registry.get('orders').event, 'order.created');
  assert.equal(registry.find('nope'), undefined);
  assert.throws(() => registry.get('nope'), /not found/);
  assert.throws(() => registry.add({ name: 'orders' }), /already exists/);
  assert.deepEqual(registry.forEvent('order.created').map((topic) => topic.name), ['orders']);
  assert.equal(registry.names().includes('imports'), true);
  assert.match(registry.describe()[0], /^default:/);
  assert.equal(registry.count(), DEFAULT_TOPICS.length);
  registry.disable('orders');
  assert.deepEqual(registry.forEvent('order.created'), []);
  registry.enable('orders');
  assert.deepEqual(fanoutPlan(registry, 'order.created', 'ord_1'), [
    { topic: 'orders', target: 'primary', event: 'order.created', entity_id: 'ord_1', maxAttempts: undefined },
  ]);
  assert.match(topicSummary(registry)[0], /default=/);
  assert.equal(registry.remove('orders').name, 'orders');
});

test('allowlist entries are hostnames, never urls', () => {
  assert.equal(validateEntry(' Example.COM. '), 'example.com');
  assert.equal(validateEntry('*.example.com'), '*.example.com');
  for (const bad of ['https://example.com', 'example.com/path', 'user@example.com', 'a*b.com', '', 'localhost']) {
    assert.throws(() => validateEntry(bad), undefined, bad);
  }
  assert.deepEqual(parseAllowlist('a.test, b.test,a.test,,'), ['a.test', 'b.test']);
  assert.deepEqual(parseAllowlist(undefined), []);
  assert.deepEqual(allowlistFromEnv('a.test'), ['a.test']);
});

test('allowlist matching covers the host, subdomains and a wildcard', () => {
  assert.equal(matchesEntry('example.com', 'example.com'), true);
  assert.equal(matchesEntry('cdn.example.com', 'example.com'), true);
  assert.equal(matchesEntry('notexample.com', 'example.com'), false);
  assert.equal(matchesEntry('a.example.com', '*.example.com'), true);
  assert.equal(matchesEntry('example.com', '*.example.com'), true);
  assert.equal(matchesEntry('b.other.com', '*.example.com'), false);
  assert.equal(isAllowlisted('cdn.example.com', ['example.com']), true);
  assert.equal(describeAllowlist(['a.test']), 'a.test');
  assert.equal(describeAllowlist([]), 'any public host');
  assert.deepEqual(diffAllowlist(['a.test', 'b.test'], ['b.test', 'c.test']), {
    added: ['c.test'],
    removed: ['a.test'],
  });
});

test('key rotation reads the age of each key', () => {
  const now = new Date('2026-03-01T00:00:00Z');
  const fresh = { id: 'k1', active: true, created_at: '2026-02-28T00:00:00Z' };
  const old = { id: 'k2', active: true, created_at: '2026-01-01T00:00:00Z' };
  assert.equal(ageDays(fresh, now), 1);
  assert.ok(ageOf(old, now) > 0);
  assert.equal(rotationDue(fresh, DEFAULT_ROTATION, now), false);
  assert.equal(rotationDue(old, DEFAULT_ROTATION, now), true);
  assert.equal(withinGrace(old, DEFAULT_ROTATION, now), false);
  const plan = planRotation([fresh, old], DEFAULT_ROTATION, now);
  assert.equal(plan.rotate, true);
  assert.deepEqual(plan.due, ['k2']);
  assert.deepEqual(plan.expired, ['k2']);
  assert.equal(plan.reason, 'age');
  assert.equal(plan.active, 2);
  assert.equal(usableKeys([fresh, old], DEFAULT_ROTATION, now).length, 1);
  assert.match(rotationSummary([fresh], DEFAULT_ROTATION, now), /reason=fresh/);
  const retired = { id: 'k3', active: false, created_at: '2025-01-01T00:00:00Z' };
  assert.deepEqual(planRotation([retired], DEFAULT_ROTATION, now).due, []);
  assert.throws(() => ageOf({}), /created_at/);
});

test('the audit log keeps the newest entries within its limit', () => {
  const log = new AuditLog({ clock: () => new Date('2026-01-01T00:00:00Z'), limit: 3 });
  log.record('queued', { delivery_id: 'dlv_1', host: 'a.test' });
  log.record('attempted', { delivery_id: 'dlv_1' });
  log.record('delivered', { delivery_id: 'dlv_1', host: 'a.test' });
  log.record('failed', { delivery_id: 'dlv_2', detail: 'boom' });
  assert.equal(log.count(), 3);
  assert.equal(log.recent(1)[0].action, 'failed');
  assert.equal(log.byAction('attempted').length, 1);
  assert.equal(log.counts().failed, 1);
  assert.match(auditSummary(log), /failed=1/);
  assert.equal(log.tail(2).length, 2);
  assert.throws(() => log.record('nope'), /unknown audit action/);
  log.clear();
  assert.equal(log.count(), 0);
});

test('payload envelopes are sized, digested and redacted', () => {
  assert.equal(payloadBytes({ a: 1 }), 7);
  assert.equal(assertPayloadSize({ a: 1 }), 7);
  assert.throws(() => assertPayloadSize({ a: 'x'.repeat(70_000) }), /at most 65536/);
  const env = envelope('order.created', { id: 'ord_1' }, { topic: 'orders', now: Date.UTC(2026, 0, 1) });
  assert.equal(env.event, 'order.created');
  assert.equal(env.sent_at, '2026-01-01T00:00:00.000Z');
  assert.equal(env.topic, 'orders');
  assert.equal(digestOf(env.data), digestOf({ id: 'ord_1' }));
  assert.equal(sameEnvelope({ data: { a: 1 } }, { data: { a: 1 } }), true);
  assert.equal(sameEnvelope({ data: { a: 1 } }, { data: { a: 2 } }), false);
  assert.equal(parseEnvelope(JSON.stringify(env)).event, 'order.created');
  assert.throws(() => parseEnvelope('[]'), /envelope must be a JSON object/);
  assert.throws(() => parseEnvelope('{"data":1}'), /needs an event/);
  assert.equal(redactEnvelope({ token: 'x', ok: 1 }).token, '[redacted]');
  assert.match(envelopeSummary(env), /order.created/);
});

import { DeliveryStore, deadLetter, deliveryAge, isStale } from '../src/queue/store.js';
import {
  attemptBudget,
  backoffTable,
  describePolicy,
  giveUpAfter,
  retrySummary,
  withJitter,
} from '../src/jobs/retry.js';
import { asArray, deepKeys, isJsonObject, jsonPath, sizeReport } from '../src/util/json.js';
import { describeLimits, isRetryable, mergeConfig, windowSeconds } from '../src/config.js';
import { findTopicForTarget, mergeTopic, planFrom } from '../src/jobs/topics.js';
import { describeRules, isPublicTarget, withAllowlist } from '../src/egress/rules.js';
import { EgressRules } from '../src/egress/rules.js';
import { createRelay, sampleDelivery } from '../src/relay.js';
import {
  arrivalRate,
  attemptHistogram,
  attemptStats,
  compareDigests,
  compareReports,
  deliveryDigest,
  digestLines,
  digestSummary,
  errorCounts,
  failureWindows,
  groupBy,
  hostCounts,
  hostTable,
  percentile,
  renderDigest,
  renderHostTable,
  renderReport,
  reportFor,
  staleness,
  statusCounts,
  successRate,
  timeline,
  topErrors,
  topicCounts,
  trend,
} from '../src/observability/report.js';

function deliveryRow(overrides = {}) {
  return {
    id: 'dlv_1',
    event: 'order.created',
    topic: 'orders',
    target: 'hooks.example.com',
    status: 'delivered',
    attempts: 1,
    created_at: '2026-01-01T10:00:00.000Z',
    updated_at: '2026-01-01T10:00:00.000Z',
    last_error: null,
    ...overrides,
  };
}

test('the store answers by entity, term and age', () => {
  const db = new DeliveryStore({ clock: () => new Date('2026-01-01T10:05:00Z') });
  const a = db.insert({ event: 'order.created', topic: 'orders', url: 'https://a.test/x', entity_id: 'ord_1' });
  db.insert({ event: 'item.updated', topic: 'items', url: 'https://b.test/x', entity_id: 'itm_1' });
  assert.equal(db.byEntity('ord_1').length, 1);
  assert.equal(db.search('ORDER').length, 1);
  assert.deepEqual(db.search(''), []);
  assert.equal(db.stale(0).length, 2);
  assert.deepEqual(db.requeueStale(0).sort(), db.list().map((row) => row.id).sort());
  const summary = db.summary();
  assert.equal(summary.counts.total, 2);
  assert.deepEqual(summary.topics, ['items', 'orders']);
  assert.equal(summary.hosts.length, 2);
  const seen = { id: 'dlv_seen', status: 'pending', updated_at: '2026-01-01T10:00:00.000Z' };
  assert.equal(deliveryAge(seen, new Date('2026-01-01T10:05:00Z')), 300);
  assert.equal(isStale(seen, 300, new Date('2026-01-01T10:05:00Z')), true);
  assert.equal(isStale(seen, 301, new Date('2026-01-01T10:05:00Z')), false);
  db.markFailed(a.id, new Error('boom'));
  assert.equal(db.deadLetters().length, 1);
  assert.deepEqual(deadLetter(db.get(a.id)).attempts, 0);
});

test('retry helpers describe the budget', () => {
  const config = { maxAttempts: 3 };
  assert.deepEqual(backoffTable(config, 3).map((row) => row.delayMs), [250, 500, 1000]);
  assert.equal(attemptBudget(config), 750);
  assert.equal(describePolicy(config), 'maxAttempts=3 base=250ms max=30000ms');
  assert.equal(withJitter(1000, () => 1), 1000);
  assert.equal(withJitter(1000, () => 0), 500);
  assert.equal(giveUpAfter({ retryable: false }, 1, config), true);
  assert.equal(giveUpAfter({}, 3, config), true);
  assert.equal(giveUpAfter({}, 1, config), false);
  assert.match(retrySummary({ id: 'dlv_1', status: 'retrying', attempts: 1 }), /attempts=1/);
  assert.equal(retrySummary(undefined), 'unknown delivery');
});

test('json helpers walk and size a document', () => {
  const value = { a: { b: [1, 2] }, c: true };
  assert.equal(jsonPath(value, 'a.b'), value.a.b);
  assert.equal(jsonPath(value, 'a.nope'), undefined);
  assert.equal(jsonPath(value, ''), value);
  assert.deepEqual(deepKeys(value), ['a', 'a.b', 'a.b.0', 'a.b.1', 'c']);
  const size = sizeReport(value);
  assert.equal(size.keys, 5);
  assert.equal(size.depth, 3);
  assert.ok(size.bytes > 0);
  assert.equal(isJsonObject(value), true);
  assert.equal(isJsonObject([]), false);
  assert.deepEqual(asArray(1), [1]);
  assert.deepEqual(asArray(undefined), []);
});

test('configuration helpers compose', () => {
  const config = mergeConfig({ concurrency: 2 }, { batchSize: 5 });
  assert.equal(config.concurrency, 2);
  assert.equal(config.batchSize, 5);
  assert.equal(windowSeconds(config), 1439 * 60);
  assert.equal(isRetryable({ retryable: true }), true);
  assert.equal(isRetryable({ retryable: false }), false);
  assert.match(describeLimits(config), /rows=5/);
});

test('topics can be patched and planned', () => {
  const registry = new TopicRegistry(DEFAULT_TOPICS);
  const merged = mergeTopic(registry, 'orders', { maxAttempts: 2 });
  assert.equal(merged.maxAttempts, 2);
  assert.equal(registry.get('orders').maxAttempts, 2);
  const plan = planFrom(registry, 'order.created', 'ord_1', { dryRun: true, limit: 1 });
  assert.equal(plan.length, 1);
  assert.equal(plan[0].dry_run, true);
  assert.equal(findTopicForTarget(registry, 'hooks').name, 'imports');
});

test('egress rules can be copied and described', () => {
  const rules = new EgressRules({ allowlist: ['example.com'] });
  const copy = withAllowlist(rules, ['other.test']);
  assert.equal(copy.allowsHost('other.test'), true);
  assert.equal(copy.allowsHost('example.com'), false);
  assert.equal(describeRules(rules), 'https: hosts=example.com internal=blocked');
  assert.equal(isPublicTarget(rules, 'example.com'), true);
  assert.equal(isPublicTarget(rules, '127.0.0.1'), false);
});

test('the digest counts what happened to each delivery', () => {
  const rows = [
    deliveryRow(),
    deliveryRow({ id: 'dlv_2', status: 'failed', attempts: 5, target: 'a.test', last_error: { code: 'delivery_failed' } }),
    deliveryRow({ id: 'dlv_3', status: 'retrying', attempts: 2, target: 'a.test', last_error: { code: 'timeout' } }),
    deliveryRow({ id: 'dlv_4', status: 'pending', attempts: 0, topic: 'items', target: 'b.test', created_at: '2026-01-01T10:01:00.000Z' }),
  ];
  assert.deepEqual(statusCounts(rows), { delivered: 1, failed: 1, retrying: 1, pending: 1 });
  assert.equal(groupBy(rows, (row) => row.target).size, 3);
  assert.equal(hostCounts(rows)[0].host, 'a.test');
  assert.deepEqual(topicCounts(rows).map((entry) => entry.topic), ['items', 'orders']);
  assert.equal(successRate({ delivered: 3, failed: 1 }), 75);
  assert.equal(successRate({}), 0);
  assert.deepEqual(attemptHistogram(rows), { '0': 1, '1': 1, '2': 1, '5': 1 });
  assert.deepEqual(errorCounts(rows), { delivery_failed: 1, timeout: 1 });
  assert.equal(topErrors(rows)[0].count, 1);
  assert.equal(timeline(rows).length, 2);
  assert.equal(percentile([5, 1, 3], 0.5), 3);
  assert.equal(percentile([], 0.5), 0);
  assert.deepEqual(attemptStats(rows), { total: 8, max: 5, p50: 1, p95: 5 });
});

test('a delivery report renders and compares', () => {
  const rows = [deliveryRow(), deliveryRow({ id: 'dlv_2', status: 'failed', attempts: 3, target: 'a.test' })];
  const digest = deliveryDigest(rows, { now: '2026-01-01T11:00:00Z' });
  assert.equal(digest.total, 2);
  assert.equal(digest.success_rate, 50);
  assert.equal(digest.seen_at, '2026-01-01T11:00:00.000Z');
  assert.match(renderDigest(digest), /deliveries: 2/);
  assert.match(digestSummary(digest), /2 deliveries/);
  assert.deepEqual(digestLines(digest).slice(0, 3).length, 3);
  const changed = compareDigests(digest, { ...digest, total: 3, statuses: { delivered: 3, failed: 1 }, success_rate: 75 });
  assert.equal(changed.total, 1);
  assert.equal(changed.success_rate, 25);
  assert.equal(changed.changed, true);
  assert.equal(compareDigests(digest, digest).changed, false);
  const report = reportFor(rows, { now: '2026-01-01T11:00:00Z', staleSeconds: 60 });
  assert.equal(report.stale.length, 0);
  assert.equal(report.timeline.length, 1);
  assert.match(renderReport(report), /deliveries: 2/);
  const hosts = hostTable(rows);
  assert.equal(hosts.find((entry) => entry.host === 'hooks.example.com').success_rate, 100);
  assert.equal(hosts.find((entry) => entry.host === 'a.test').success_rate, 0);
  assert.match(renderHostTable(rows), /^host,rows,delivered,failed,success_rate/);
  assert.equal(failureWindows(rows).length, 1);
  assert.deepEqual(compareReports(report, report).digest.changed, false);
  assert.deepEqual(trend(digest, { success_rate: 75 }), { delta: 25, direction: 'up' });
  assert.deepEqual(trend(digest, { success_rate: 25 }).direction, 'down');
  assert.equal(arrivalRate(rows, { from: rows[0].created_at, to: rows[1].created_at }), 2);
});

test('stale deliveries are aged against the clock', () => {
  const rows = [deliveryRow({ id: 'a', status: 'pending', updated_at: '2026-01-01T09:00:00.000Z' })];
  const stale = staleness(rows, { now: '2026-01-01T10:00:00Z', staleSeconds: 300 });
  assert.equal(stale.length, 1);
  assert.equal(stale[0].age_seconds, 3600);
  assert.equal(staleness(rows, { now: '2026-01-01T09:01:00Z', staleSeconds: 300 }).length, 0);
});

test('the relay exposes its digest, dead letters and audit tail', async () => {
  const app = createRelay({
    clock: () => new Date('2026-01-01T00:00:00Z'),
    transport: async () => ({ status: 500 }),
    config: { maxAttempts: 1 },
  });
  app.bulkEnqueue([sampleDelivery({ entity_id: 'a' }), sampleDelivery({ entity_id: 'b' })]);
  const rows = await app.runOnce();
  assert.equal(rows.length, 2);
  assert.equal(app.deadLetters().length, 2);
  assert.equal(app.reprocessDeadLetters().length, 2);
  assert.equal(app.digest().digest.total, 2);
  assert.match(app.digestText(), /deliveries/);
  assert.ok(app.auditTail(5).length > 0);
  assert.equal(app.topicsFor('order.created')[0].name, 'orders');
  assert.throws(() => app.bulkEnqueue('nope'), /expects a list/);
});
