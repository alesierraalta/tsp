import test from 'node:test';
import assert from 'node:assert/strict';
import { Session } from '../src/session.js';
import { MemoryEndpoint } from '../src/endpoint.js';
import { Receipts } from '../src/receipts.js';
import { Journal } from '../src/journal.js';
import { chunks, assemble, digest } from '../src/chunks.js';
import { options } from '../src/options.js';
import { retry, TransferError, summarize } from '../src/retry.js';

test('delivers queued documents in order and exposes receipts', async () => {
  const endpoint = new MemoryEndpoint();
  const session = new Session(endpoint, { chunkSize: 2, namespace: 'team' });
  session.enqueue('alpha', 'hello');
  session.enqueue('beta', 'world');
  assert.deepEqual(session.journal.snapshot().map(x => x.id), ['alpha', 'beta']);
  assert.equal(session.journal.get('alpha').bytes.toString(), 'hello');
  assert.equal(session.report(0, 1).next, 1);
  assert.equal(session.report().totals.pending, 2);
  assert.throws(() => session.report(-1));
  assert.throws(() => session.report(0, 0));
  assert.equal((await session.flush()).length, 2);
  assert.equal(endpoint.read('team/alpha').toString(), 'hello');
  assert.equal(endpoint.read('team/beta').toString(), 'world');
  assert.deepEqual(session.status(), { pending: 0, delivered: 2, busy: false });
  assert.equal(session.receipt('alpha').bytes, 5);
  assert.equal(session.receipts.list().length, 2);
  assert.equal(session.receipts.stats().bytes, 10);
  assert.equal(session.report().totals.delivered, 2);
  assert.equal(session.report().text, 'alpha: delivered (5 bytes)\nbeta: delivered (5 bytes)');
  assert.deepEqual(endpoint.keys(), ['team/alpha', 'team/beta']);
  assert.equal(endpoint.stats().bytesReceived, 10);
  assert.deepEqual(session.transport.stats(), { attempts: 2, succeeded: 2, failed: 0 });
  assert.equal(session.transport.history('team/alpha').length, 1);
  assert.equal(session.transport.clearHistory(), 2);
  assert.deepEqual(await session.flush(), []);
});

test('chunks preserve bytes, ownership, and deterministic summaries', () => {
  const bytes = Buffer.from('café');
  const parts = chunks(bytes, 2);
  assert.deepEqual(assemble(parts), bytes);
  parts[0][0] = 0;
  assert.equal(bytes[0], 99);
  assert.equal(digest(bytes), digest(Buffer.from('café')));
  assert.deepEqual(new Session(new MemoryEndpoint()).preview('abc'), {
    bytes: 3, parts: 1, checksum: digest(Buffer.from('abc')), largest: 3,
  });
  assert.deepEqual(chunks(Buffer.alloc(0), 2), []);
  assert.throws(() => chunks(bytes, 0));
  assert.throws(() => chunks('abc', 2));
});

test('validates queue inputs and keeps copies independent', () => {
  assert.throws(() => options({ attempts: 0 }));
  assert.throws(() => options({ namespace: '' }));
  const session = new Session(new MemoryEndpoint(), { maxBytes: 3 });
  assert.throws(() => session.enqueue('bad/id', 'a'));
  assert.throws(() => session.enqueue('a', 'long'));
  assert.throws(() => session.enqueue('a', 4));
  session.enqueue('a', 'yes');
  assert.throws(() => session.enqueue('a', 'yes'));
  const copy = session.journal.get('a');
  copy.bytes[0] = 0;
  assert.equal(session.journal.get('a').bytes.toString(), 'yes');
  const journal = new Journal();
  assert.equal(journal.get('missing'), null);
  assert.equal(journal.acknowledge('missing'), false);
});

test('policy retries transient failures and stops terminal errors', async () => {
  const events = [];
  const result = await retry(async n => {
    if (n < 3) throw new TransferError('unavailable');
    return 7;
  }, 3, e => events.push(e));
  assert.equal(result, 7);
  assert.deepEqual(summarize(events), { attempts: 3, succeeded: 1, failed: 2 });
  await assert.rejects(retry(async () => { throw new TransferError('denied', false); }, 3));
  await assert.rejects(retry(async () => { throw new Error('invalid'); }, 3));
  await assert.rejects(retry(async () => 1, 0));
});

test('receipt repository commits only after its storage adapter accepts', async () => {
  let fail = true;
  const receipts = new Receipts(async () => { if (fail) throw new Error('storage'); });
  const row = { key: 'x', bytes: 1, checksum: 'a' };
  await assert.rejects(receipts.save('x', row));
  assert.equal(receipts.get('x'), null);
  fail = false;
  await receipts.save('x', row);
  const copy = receipts.get('x');
  copy.bytes = 9;
  assert.equal(receipts.get('x').bytes, 1);
  await assert.rejects(receipts.save('x', { ...row, checksum: 'b' }));
});

test('rejects overlapping flushes and duplicate delivered identifiers', async () => {
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  const endpoint = new MemoryEndpoint(async () => gate);
  const session = new Session(endpoint);
  session.enqueue('a', 'ok');
  const first = session.flush();
  await assert.rejects(session.flush());
  release();
  await first;
  assert.throws(() => session.enqueue('a', 'ok'));
  assert.equal(endpoint.read('none'), null);
});
