import test from 'node:test';
import assert from 'node:assert/strict';
import { SeatPool, ProfileCache } from '../src/coordinator.js';

test('assigns available seats', async () => {
  const pool = new SeatPool(1, async () => {});
  assert.equal(await pool.reserve('alice'), true);
  assert.equal(await pool.reserve('bob'), false);
  assert.equal(pool.available, 0);
});

test('authorization errors propagate', async () => {
  const pool = new SeatPool(1, async () => { throw new Error('denied'); });
  await assert.rejects(pool.reserve('alice'), /denied/);
  assert.equal(pool.available, 1);
});

test('reuses a loaded profile', async () => {
  let calls = 0;
  const cache = new ProfileCache(async key => { calls++; return { key }; });
  const first = await cache.get('alice');
  assert.equal(await cache.get('alice'), first);
  assert.equal(calls, 1);
});

test('can retry an unavailable profile', async () => {
  let calls = 0;
  const cache = new ProfileCache(async () => {
    if (++calls === 1) throw new Error('unavailable');
    return 'ready';
  });
  await assert.rejects(cache.get('alice'), /unavailable/);
  assert.equal(await cache.get('alice'), 'ready');
});
