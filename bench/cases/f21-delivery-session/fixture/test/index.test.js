import test from 'node:test';
import assert from 'node:assert/strict';
import { deliver, request } from '../src/index.js';

test('delivery returns the remote receipt', async () => {
  assert.equal(await deliver('order-1', 7, async (key, value) => `${key}=${value}`), 'order-1=7');
});

test('delivery accepts a subsequent response', async () => {
  let calls = 0;
  assert.equal(await deliver('order-2', 9, async () => {
    if (++calls === 1) throw new Error('offline');
    return 'accepted';
  }), 'accepted');
  assert.equal(calls, 2);
});

test('request uses the available allowance', async () => {
  let clock = 0;
  let calls = 0;
  const allowances = [];
  assert.equal(await request(20, () => clock, async remaining => {
    allowances.push(remaining);
    clock += 3;
    if (++calls === 1) throw new Error('offline');
    return 'ready';
  }), 'ready');
  assert.deepEqual(allowances, [20, 17]);
});

test('request rejects unsuccessful work', async () => {
  await assert.rejects(request(20, () => 0, async () => { throw new Error('offline'); }, 1), /offline/);
});
