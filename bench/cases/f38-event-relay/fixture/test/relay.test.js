import test from 'node:test';
import assert from 'node:assert/strict';
import { MessageLog, LogClient } from '../lib/message-log.js';
import { checkSignal, checkpoint, delay } from '../lib/signal.js';
import { Relay } from '../src/relay.js';
import { PermitPool } from '../src/permits.js';
import { encode, headers, pipeFrames } from '../src/frames.js';

test('stored events resume and acknowledge on close', async () => {
  const log = new MessageLog();
  const payload = { value: 1 };
  log.append('news', payload);
  payload.value = 8;
  log.append('news', { value: 2 });
  const client = new LogClient(log);
  const relay = new Relay(client, { batchSize: 1 });
  const first = relay.subscribe('desk', 'news');
  const frame = (await first.next()).value;
  assert.equal(frame.payload.value, 1);
  assert.match(encode(frame), /id: 1/);
  await first.return();
  assert.equal(client.acknowledged.get('desk'), 1);
  const second = relay.subscribe('desk', 'news', { cursor: 1 });
  assert.equal((await second.next()).value.cursor, 2);
  await second.return();
  assert.equal(relay.status('desk').permits, 0);
  assert.equal(relay.status('desk').events, 2);
  assert.deepEqual(log.topics(), ['news']);
  assert.equal(log.size('news'), 2);
  assert.equal(log.after('news', 0, 1).length, 1);
});

test('quiet transport emits frames and preserves cursors', async () => {
  const calls = [];
  const client = {
    async read(topic, cursor, options) {
      calls.push({ topic, cursor, limit: options.limit });
      return [];
    },
    async acknowledge(subscriber, cursor) { calls.push({ subscriber, cursor }); },
  };
  const relay = new Relay(client);
  const stream = relay.subscribe('viewer', 'updates', { cursor: 7 });
  assert.deepEqual((await stream.next()).value, { type: 'keep-alive', cursor: 7 });
  assert.equal(encode((await stream.next()).value), ': keep-alive 7\n\n');
  await stream.return();
  assert.equal(relay.status('viewer').keepAlives, 2);
  assert.equal(relay.status('viewer').active, 0);
  assert.equal(calls[0].limit, 32);
  assert.equal(calls.at(-1).cursor, 7);
});

test('independent subscribers and idempotent permit returns', () => {
  const pool = new PermitPool();
  const a = pool.acquire('a');
  const b = pool.acquire('b');
  assert.throws(() => pool.acquire('a'), /capacity/);
  assert.deepEqual(pool.snapshot(), { a: 1, b: 1 });
  a(); a(); b();
  assert.deepEqual(pool.snapshot(), {});
  assert.throws(() => new PermitPool(0));
});

test('validation and cancellation before iteration', async () => {
  const relay = new Relay(new LogClient(new MessageLog()));
  assert.throws(() => relay.subscribe('', 'news'));
  assert.throws(() => relay.subscribe('a', 'news', { cursor: -1 }));
  assert.throws(() => new Relay({}, { batchSize: 0 }));
  const controller = new AbortController();
  const stream = relay.subscribe('a', 'news', { signal: controller.signal });
  controller.abort(new Error('closed'));
  await stream.return();
  assert.equal(relay.status('a').permits, 0);
  assert.throws(() => checkSignal(controller.signal), /closed/);
  await assert.rejects(checkpoint(controller.signal), /closed/);
  await delay(1);
});

test('text adapter closes its sink', async () => {
  const chunks = [];
  async function* frames() {
    yield { type: 'event', topic: 'news', cursor: 4, payload: 'hello' };
  }
  let ended = false;
  const written = await pipeFrames(frames(), {
    async write(text) { chunks.push(text); },
    async end() { ended = true; },
  });
  assert.equal(written, 1);
  assert.equal(ended, true);
  assert.match(chunks[0], /event: news/);
  assert.equal(headers()['content-type'], 'text/event-stream');
});
