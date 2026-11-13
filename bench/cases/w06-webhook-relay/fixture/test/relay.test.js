import test from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { createRelay, Relay, sampleDelivery } from '../src/relay.js';
import { HookRegistry } from '../src/jobs/registry.js';
import { EgressRules } from '../src/egress/rules.js';
import { Metrics, mergeMetrics, summarize } from '../src/observability/metrics.js';
import { describeConfig, configFromEnv, retryDelay, shouldRetry } from '../src/config.js';
import {
  canonicalJson,
  jsonLines,
  jsonSize,
  mergeJson,
  parseJson,
  pickJson,
  redactJson,
  truncateJson,
} from '../src/util/json.js';
import { isRelayError, RelayError, asRelayError } from '../src/util/errors.js';
import { canonical, maskSecret, redactHeaders, trimTo } from '../src/util/text.js';

const TAG_SCRIPT = fileURLToPath(new URL('../hooks/tag.mjs', import.meta.url));

function relay(overrides = {}) {
  return createRelay({
    clock: () => new Date('2026-01-01T00:00:00Z'),
    transport: async () => ({ status: 200 }),
    ...overrides,
  });
}

test('enqueue validates the target and stores the delivery', () => {
  const app = relay();
  const row = app.enqueue(sampleDelivery());
  assert.match(row.id, /^dlv_/);
  assert.equal(row.url, 'https://hooks.example.com/relay');
  assert.equal(row.topic, 'orders');
  assert.equal(app.store.count(), 1);
  assert.equal(app.stats().due, 1);
  assert.equal(app.metrics.counter('deliveries.queued'), 1);
  assert.equal(app.pending().length, 1);
});

test('enqueue refuses an internal target and a host outside the allowlist', () => {
  const app = relay();
  assert.throws(() => app.enqueue(sampleDelivery({ url: 'https://127.0.0.1/admin' })), /internal address/);
  assert.equal(app.store.count(), 0);
  const closed = relay({ config: { allowlist: ['example.com'] } });
  assert.throws(() => closed.enqueue(sampleDelivery({ url: 'https://other.test/hook' })), /allowlist/);
  assert.equal(closed.enqueue(sampleDelivery({ url: 'https://cdn.example.com/hook' })).topic, 'orders');
});

test('enqueue refuses an equivalent queued delivery', () => {
  const app = relay();
  app.enqueue(sampleDelivery());
  assert.throws(() => app.enqueue(sampleDelivery()), /equivalent delivery/);
});

test('deliverNow posts a stored delivery and records the receipt', async () => {
  const app = relay({ transport: async () => ({ status: 202 }) });
  const row = app.store.insert(sampleDelivery());
  const receipt = await app.deliverNow(row.id);
  assert.equal(receipt.status, 202);
  assert.equal(app.store.get(row.id).status, 'delivered');
  assert.equal(app.metrics.counter('deliveries.delivered'), 1);
});

test('deliverNow marks a failed delivery and rethrows', async () => {
  const app = relay({ transport: async () => ({ status: 500 }) });
  const row = app.store.insert(sampleDelivery());
  await assert.rejects(app.deliverNow(row.id), /answered 500/);
  assert.equal(app.store.get(row.id).status, 'failed');
  assert.equal(app.store.get(row.id).attempts, 1);
});

test('retryNow requeues a delivery that has not landed', () => {
  const app = relay();
  const row = app.store.insert(sampleDelivery());
  app.store.markFailed(row.id, new Error('boom'));
  const retried = app.retryNow(row.id);
  assert.equal(retried.status, 'pending');
  assert.equal(app.store.due(app.clock()).length, 1);
  app.store.markDelivered(row.id, { status: 200 });
  assert.throws(() => app.retryNow(row.id), /not found/);
});

test('a drain delivers everything due', async () => {
  const app = relay();
  app.enqueue(sampleDelivery({ entity_id: 'a' }));
  app.enqueue(sampleDelivery({ entity_id: 'b' }));
  const results = await app.drain(3);
  assert.equal(results.length, 2);
  assert.equal(app.store.count('delivered'), 2);
  assert.deepEqual(await app.drain(2), []);
});

test('the import hook runs through the relay', async () => {
  const hooks = new HookRegistry([{ name: 'tag', event: 'import.finished', script: TAG_SCRIPT }]);
  const app = relay({ hooks });
  const result = await app.hookForImport('payload.json');
  assert.equal(result.stdout, 'tagged payload.json');
  assert.equal(app.stats().hooks, 1);
  assert.match(app.describe().hooks[0], /tag:import.finished/);
  assert.equal(app.describe().egress, 'any public host');
});

test('relay stats count the queue and the metrics', async () => {
  const app = relay();
  app.enqueue(sampleDelivery({ entity_id: 'a' }));
  app.enqueue(sampleDelivery({ entity_id: 'b' }));
  await app.runOnce();
  const stats = app.stats();
  assert.equal(stats.counts.total, 2);
  assert.equal(stats.counts.delivered, 2);
  assert.equal(stats.metrics.counters['deliveries.attempted'], 2);
});

test('the relay can be built from parts', () => {
  const app = relay();
  assert.ok(app instanceof Relay);
  assert.equal(app.rules instanceof EgressRules, true);
  assert.equal(app.offsets.count(), 0);
  assert.ok(app.config.maxAttempts >= 1);
});

test('configuration is validated and rendered', () => {
  const config = configFromEnv({ RELAY_CONCURRENCY: '8', RELAY_ALLOWLIST: 'a.test, b.test' });
  assert.equal(config.concurrency, 8);
  assert.deepEqual(config.allowlist, ['a.test', 'b.test']);
  assert.match(describeConfig(config), /concurrency=8/);
  assert.equal(retryDelay(3, config), 1000);
  assert.equal(shouldRetry({}, 1, config), true);
  assert.equal(shouldRetry({ retryable: false }, 1, config), false);
  assert.equal(shouldRetry({}, config.maxAttempts, config), false);
  assert.throws(() => loadBad(), /concurrency/);
});

function loadBad() {
  return relay({ config: { concurrency: 0 } });
}

test('metrics count, gauge, time and render', async () => {
  const metrics = new Metrics({ clock: (() => {
    let now = 0;
    return () => (now += 5);
  })() });
  metrics.increment('a');
  metrics.increment('a', 2);
  metrics.gauge('q', 3);
  metrics.observe('t', 10);
  metrics.observe('t', 20);
  await metrics.time('work', async () => 'done');
  const snapshot = metrics.snapshot();
  assert.equal(snapshot.counters.a, 3);
  assert.equal(snapshot.gauges.q, 3);
  assert.equal(snapshot.timings.t.count, 2);
  assert.equal(snapshot.timings.t.mean, 15);
  assert.equal(metrics.counter('a'), 3);
  assert.match(metrics.render(), /counter a 3/);
  assert.deepEqual(summarize([]), { count: 0, mean: 0, min: 0, max: 0 });
  const target = new Metrics();
  mergeMetrics(target, metrics);
  assert.equal(target.counter('a'), 3);
  metrics.reset();
  assert.equal(metrics.counter('a'), 0);
});

test('json helpers keep payloads readable and redacted', () => {
  assert.deepEqual(parseJson('{"a":1}', {}), { a: 1 });
  assert.deepEqual(parseJson('{bad', { fallback: true }), { fallback: true });
  assert.equal(canonicalJson({ b: 1, a: 2 }), '{"a":2,"b":1}');
  assert.equal(canonicalJson([{ b: 1 }]), '[{"b":1}]');
  assert.deepEqual(redactJson({ token: 'x', nested: { password: 'y' } }), {
    token: '[redacted]',
    nested: { password: '[redacted]' },
  });
  assert.deepEqual(mergeJson({ a: { b: 1 } }, { a: { c: 2 } }), { a: { b: 1, c: 2 } });
  assert.deepEqual(pickJson({ a: 1, b: 2 }, ['a']), { a: 1 });
  assert.equal(jsonSize({ a: 1 }), 7);
  assert.ok(truncateJson({ a: 'x'.repeat(9000) }, 100).endsWith('...[truncated]'));
  assert.equal(jsonLines([{ a: 1 }, { b: 2 }]), '{"a":1}\n{"b":2}');
});

test('text and error helpers behave', () => {
  assert.equal(canonical('  ABC '), 'abc');
  assert.equal(trimTo('abcdef', 4), 'abc\u2026');
  assert.equal(maskSecret('abcdefgh', 2), '******gh');
  assert.deepEqual(redactHeaders({ authorization: 'x', 'x-other': 'y' }), {
    authorization: '[redacted]',
    'x-other': 'y',
  });
  const error = new RelayError('not_found', 'nope');
  assert.equal(isRelayError(error), true);
  assert.equal(isRelayError(new Error('x')), false);
  assert.equal(asRelayError(new Error('x')).code, 'internal_error');
  assert.equal(asRelayError(error), error);
  assert.deepEqual(error.toJSON(), { error: 'not_found', message: 'nope' });
});
