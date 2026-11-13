import test from 'node:test';
import assert from 'node:assert/strict';
import { EgressRules } from '../src/egress/rules.js';
import { WebhookClient, DEFAULT_HEADERS } from '../src/egress/webhook.js';
import { KeyStore, createKey, maskedKey } from '../src/auth/keys.js';
import {
  parseSignatureHeader,
  secretLabel,
  signBody,
  signMessage,
  verifyMessage,
} from '../src/auth/signature.js';
import { DeliveryError, EgressError } from '../src/util/errors.js';

function client(overrides = {}) {
  return new WebhookClient({
    rules: new EgressRules(),
    transport: async () => ({ status: 200 }),
    ...overrides,
  });
}

test('a public delivery is prepared with default headers', () => {
  const plan = client().prepare({ event: 'x', url: 'https://hooks.example.com/a', payload: { a: 1 } });
  assert.equal(plan.host, 'hooks.example.com');
  assert.equal(plan.headers['content-type'], DEFAULT_HEADERS['content-type']);
  assert.equal(typeof plan.key, 'string');
});

test('a classified internal target is refused before any post', () => {
  const rules = new EgressRules({ allowInsecure: true });
  const webhook = client({ rules });
  for (const url of ['https://127.0.0.1/admin', 'https://10.1.2.3/admin', 'http://localhost:9/x']) {
    assert.throws(() => webhook.prepare({ url }), EgressError, url);
  }
});

test('prepare refuses a scheme the rules do not admit', () => {
  assert.throws(() => client().prepare({ url: 'http://example.com/x' }), /scheme http:/);
});

test('prepare refuses a host outside the allowlist', () => {
  const webhook = client({ rules: new EgressRules({ allowlist: ['example.com'] }) });
  assert.throws(() => webhook.prepare({ url: 'https://other.test/x' }), /allowlist/);
  assert.equal(webhook.prepare({ url: 'https://cdn.example.com/x' }).host, 'cdn.example.com');
});

test('send posts the body and reports the receipt', async () => {
  const calls = [];
  const webhook = client({
    transport: async (href, init) => {
      calls.push({ href, init });
      return { status: 201 };
    },
  });
  const receipt = await webhook.send({ event: 'x', url: 'https://hooks.example.com/a', payload: { id: 1 } });
  assert.equal(receipt.status, 201);
  assert.equal(receipt.host, 'hooks.example.com');
  assert.equal(calls.length, 1);
  assert.equal(calls[0].init.method, 'POST');
  assert.equal(calls[0].init.body, '{"id":1}');
});

test('a failed post is a retryable delivery error', async () => {
  await assert.rejects(
    client({ transport: async () => ({ status: 503 }) }).send({ url: 'https://hooks.example.com/a' }),
    (error) => {
      assert.ok(error instanceof DeliveryError);
      assert.equal(error.retryable, true);
      assert.equal(error.details.status, 503);
      return true;
    },
  );
  await assert.rejects(
    client({
      transport: async () => {
        throw new Error('socket hang up');
      },
    }).send({ url: 'https://hooks.example.com/a' }),
    /socket hang up/,
  );
});

test('signed posts carry a timestamped hmac header', () => {
  const keys = new KeyStore({ seed: createKey({ secret: 'a'.repeat(32), id: 'key_test1' }) });
  const webhook = client({ keys, clock: () => new Date('2026-01-01T00:00:00Z') });
  const plan = webhook.prepare({ url: 'https://hooks.example.com/a', payload: { a: 1 } });
  const [stamp, signature] = plan.headers['x-relay-signature'].split(',');
  assert.equal(stamp, 't=1767225600');
  assert.match(signature, /^v1=[0-9a-f]{64}$/);
  assert.equal(plan.headers['x-relay-key'], 'key_test1');
  assert.equal(
    plan.headers['x-relay-signature'],
    signMessage('{"a":1}', 'a'.repeat(32), 1767225600),
  );
});

test('a signature verifies inside the tolerance and fails outside it', () => {
  const secret = 'b'.repeat(32);
  const header = signMessage('{"a":1}', secret, 1767225600);
  assert.deepEqual(verifyMessage('{"a":1}', header, secret, { now: 1767225600 * 1000 }), {
    ok: true,
    timestamp: 1767225600,
  });
  assert.equal(
    verifyMessage('{"a":1}', header, secret, { now: 1767225600 * 1000 + 400_000 }).reason,
    'timestamp_out_of_tolerance',
  );
  assert.equal(
    verifyMessage('{"a":2}', header, secret, { now: 1767225600 * 1000 }).reason,
    'signature_mismatch',
  );
  assert.equal(signBody('x', secret).length, 64);
  assert.equal(parseSignatureHeader(header).timestamp, 1767225600);
  assert.throws(() => parseSignatureHeader('nope'), /t and v1/);
});

test('keys are masked, rotated and revoked', () => {
  const store = new KeyStore({ clock: () => new Date('2026-01-01T00:00:00Z') });
  const first = store.create({ id: 'key_a', secret: 'c'.repeat(24), label: 'first' });
  assert.equal(store.count(), 1);
  assert.equal(store.active().id, 'key_a');
  const listed = store.list();
  assert.equal(listed[0].secret.includes('c'.repeat(24)), false);
  assert.equal(listed[0].label, 'first');
  assert.equal(maskedKey(first).created_at, '2026-01-01T00:00:00.000Z');
  const rotated = store.rotate('second');
  assert.equal(store.active().id, rotated.id);
  assert.equal(store.get('key_a').active, false);
  store.revoke(rotated.id);
  assert.throws(() => store.active(), /not found/);
  assert.throws(() => store.get('key_none'), /not found/);
  assert.throws(() => store.add(first), /already exists/);
  assert.throws(() => createKey({ secret: 'short' }), /at least 16/);
  assert.equal(secretLabel('d'.repeat(20)), `${'*'.repeat(16)}dddd`);
});
