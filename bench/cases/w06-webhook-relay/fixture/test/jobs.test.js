import test from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { DeliveryStore } from '../src/queue/store.js';
import { HookRegistry, EVENTS } from '../src/jobs/registry.js';
import { Dispatcher } from '../src/jobs/dispatcher.js';
import { WebhookClient } from '../src/egress/webhook.js';
import { EgressRules } from '../src/egress/rules.js';
import { loadConfig } from '../src/config.js';
import {
  assertHookArgument,
  assertScriptPath,
  hookSummary,
  runHook,
} from '../src/hooks/runner.js';
import { hookCommand, hookPlan, postImportSummary, runPostImportHook } from '../src/jobs/hooks.js';

const TAG_SCRIPT = fileURLToPath(new URL('../hooks/tag.mjs', import.meta.url));

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

function dispatcher(options = {}) {
  const clock = () => new Date('2026-01-01T00:00:00Z');
  const store = options.store ?? new DeliveryStore({ clock });
  const rules = options.rules ?? new EgressRules();
  const client = new WebhookClient({
    rules,
    transport: options.transport ?? (async () => ({ status: 200 })),
    clock,
  });
  return new Dispatcher({
    store,
    client,
    rules,
    config: loadConfig(options.config ?? {}),
    clock,
    metrics: options.metrics,
  });
}

test('the dispatcher posts a due delivery and marks it delivered', async () => {
  const db = new DeliveryStore({ clock: () => new Date('2026-01-01T00:00:00Z') });
  const row = db.insert(delivery());
  const results = await dispatcher({ store: db }).runOnce();
  assert.equal(results[0].status, 'delivered');
  assert.equal(db.get(row.id).status, 'delivered');
  assert.equal(db.get(row.id).attempts, 1);
  assert.equal(db.get(row.id).receipt.status, 200);
});

test('a rejected post is scheduled for another attempt', async () => {
  const db = new DeliveryStore({ clock: () => new Date('2026-01-01T00:00:00Z') });
  const row = db.insert(delivery());
  const results = await dispatcher({
    store: db,
    transport: async () => ({ status: 500 }),
  }).runOnce();
  assert.equal(results[0].status, 'retrying');
  assert.equal(db.get(row.id).status, 'retrying');
  assert.equal(db.get(row.id).attempts, 1);
  assert.equal(db.get(row.id).next_attempt_at, '2026-01-01T00:00:00.250Z');
});

test('an exhausted delivery is marked failed, not retried', async () => {
  const db = new DeliveryStore({ clock: () => new Date('2026-01-01T00:00:00Z') });
  const row = db.insert(delivery());
  const results = await dispatcher({
    store: db,
    config: { maxAttempts: 1 },
    transport: async () => ({ status: 500 }),
  }).runOnce();
  assert.equal(results[0].status, 'failed');
  assert.equal(db.get(row.id).status, 'failed');
});

test('a blocked target is failed before any post and reported as blocked', async () => {
  const db = new DeliveryStore({ clock: () => new Date('2026-01-01T00:00:00Z') });
  const row = db.insert(delivery({ url: 'https://127.0.0.1/admin' }));
  let posted = false;
  const results = await dispatcher({
    store: db,
    transport: async () => {
      posted = true;
      return { status: 200 };
    },
  }).runOnce();
  assert.equal(results[0].status, 'blocked');
  assert.match(results[0].error, /internal address/);
  assert.equal(posted, false);
  assert.equal(db.get(row.id).status, 'failed');
});

test('a batch is capped by batchSize', async () => {
  const db = new DeliveryStore({ clock: () => new Date('2026-01-01T00:00:00Z') });
  db.insert(delivery({ entity_id: 'a' }));
  db.insert(delivery({ entity_id: 'b' }));
  db.insert(delivery({ entity_id: 'c' }));
  const one = await dispatcher({ store: db, config: { batchSize: 1 } }).runOnce();
  assert.equal(one.length, 1);
  const rest = await dispatcher({ store: db, config: { batchSize: 10 } }).drain(2);
  assert.equal(rest.length, 2);
  assert.equal(db.count('delivered'), 3);
});

test('no due delivery means no result', async () => {
  const db = new DeliveryStore({ clock: () => new Date('2026-01-01T00:00:00Z') });
  db.insert(delivery({ next_attempt_at: '2027-01-01T00:00:00Z' }));
  assert.deepEqual(await dispatcher({ store: db }).runOnce(), []);
  assert.deepEqual(await dispatcher({ store: db }).drain(2), []);
});

test('hook arguments are plain tokens', () => {
  assert.equal(assertHookArgument('payload.json'), 'payload.json');
  assert.equal(assertHookArgument('a-b_c.1'), 'a-b_c.1');
  for (const bad of ['a b', 'a;b', '../x', 'a/b', '', 'a`b']) {
    assert.throws(() => assertHookArgument(bad), /unsafe hook argument/, bad);
  }
  assert.throws(() => assertScriptPath('relative.mjs'), /absolute path/);
  assert.throws(() => assertScriptPath('/tmp/script.txt'), /must be a module/);
  assert.equal(assertScriptPath(TAG_SCRIPT), TAG_SCRIPT);
});

test('the guarded runner spawns the script with no shell', async () => {
  const result = await runHook(TAG_SCRIPT, ['payload.json']);
  assert.equal(result.code, 0);
  assert.equal(result.stdout, 'tagged payload.json');
  assert.equal(hookSummary(result), 'exit 0 tagged payload.json');
  await assert.rejects(runHook(TAG_SCRIPT, ['a;b']), /unsafe hook argument/);
  await assert.rejects(runHook(TAG_SCRIPT, ['../escape']), /unsafe hook argument/);
});

test('the import hook runs the registered script for a plain payload name', async () => {
  const registry = new HookRegistry([
    { name: 'tag', event: 'import.finished', script: TAG_SCRIPT },
  ]);
  const result = await runPostImportHook(registry, 'payload.json');
  assert.equal(result.stdout, 'tagged payload.json');
  assert.equal(postImportSummary(registry, 'payload.json'), 'tag <- payload.json');
  assert.match(hookPlan(registry)[0].command, /<payload>/);
});

test('an import with no hook is skipped', async () => {
  const result = await runPostImportHook(new HookRegistry(), 'payload.json');
  assert.equal(result.skipped, true);
  assert.match(result.reason, /no import.finished hook/);
});

test('the registry maps events and refuses unknown ones', () => {
  const registry = new HookRegistry();
  registry.add({ name: 'tag', event: 'import.finished', script: TAG_SCRIPT });
  registry.add({ name: 'notify', event: 'delivery.failed', script: TAG_SCRIPT });
  assert.equal(registry.forEvent('import.finished').name, 'tag');
  assert.equal(registry.forEvent('delivery.delivered'), undefined);
  assert.equal(registry.allForEvent('import.finished').length, 1);
  assert.deepEqual(registry.describe(), ['notify:delivery.failed:on', 'tag:import.finished:on']);
  assert.deepEqual(EVENTS.includes('import.finished'), true);
  registry.disable('tag');
  assert.equal(registry.forEvent('import.finished'), undefined);
  registry.enable('tag');
  assert.equal(registry.list().length, 2);
  assert.equal(registry.remove('tag').name, 'tag');
  assert.equal(registry.count(), 1);
  assert.throws(() => registry.get('nope'), /not found/);
  assert.throws(() => registry.add({ name: 'bad', event: 'nope', script: TAG_SCRIPT }), /unknown hook event/);
  assert.throws(() => registry.add({ event: 'import.finished' }), /needs a name/);
});

test('the command builder names the script and the payload', () => {
  const hook = { name: 'tag', script: '/opt/relay/hooks/tag.mjs' };
  const command = hookCommand(hook, 'payload.json');
  assert.match(command, /tag\.mjs" payload\.json$/);
  assert.match(command, /node/);
});
