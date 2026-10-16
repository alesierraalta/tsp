import test from 'node:test';
import assert from 'node:assert/strict';
import { LeasePool } from '../src/leases.js';
import { FlightRegistry } from '../src/flights.js';

test('leases serialize owners and keep keys independent', () => {
  const pool = new LeasePool();
  const ticket = pool.acquire('a');
  assert.equal(pool.acquire('a'), null);
  assert.ok(pool.acquire('b'));
  assert.equal(pool.release(ticket), true);
  const current = pool.acquire('a');
  assert.equal(pool.release(ticket), false);
  assert.equal(pool.release(current), true);
});

test('callers share pending work and settled work can run again', async () => {
  const registry = new FlightRegistry();
  let calls = 0;
  const operation = () => ++calls;
  const first = registry.run('a', operation);
  assert.equal(registry.run('a', operation), first);
  assert.equal(await first, 1);
  assert.equal(await registry.run('a', operation), 2);
});

test('failed operations free their registration', async () => {
  const registry = new FlightRegistry();
  await assert.rejects(registry.run('a', () => { throw new Error('offline'); }), /offline/);
  assert.equal(await registry.run('a', () => 'ready'), 'ready');
});

test('invalidation permits fresh work while existing callers finish', async () => {
  const registry = new FlightRegistry();
  const first = registry.run('a', () => 'old');
  registry.invalidate('a');
  const fresh = registry.run('a', () => 'fresh');
  assert.notEqual(first, fresh);
  assert.deepEqual(await Promise.all([first, fresh]), ['old', 'fresh']);
});
