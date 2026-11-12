import test from 'node:test';
import assert from 'node:assert/strict';
import {
  EnvelopeError,
  MaxBytes,
  applyEnvelope,
  coerceNumbers,
  decode,
  headerMap,
  isPlainObject,
  merge,
  pick,
} from '../src/envelope.js';

test('decode parses a json object', () => {
  assert.deepEqual(decode('{"timeoutMs":500}'), { timeoutMs: 500 });
});

test('decode refuses what is not an object', () => {
  for (const body of ['', '   ', 'null', '[]', '"text"', '7', 'not json', '{"a":']) {
    assert.throws(() => decode(body), EnvelopeError, body);
  }
});

test('decode refuses a body above the size limit', () => {
  const body = `{"endpoint":"${'x'.repeat(MaxBytes)}"}`;
  assert.throws(() => decode(body), EnvelopeError);
});

test('isPlainObject accepts objects and refuses the rest', () => {
  assert.equal(isPlainObject({}), true);
  assert.equal(isPlainObject(Object.create(null)), true);
  assert.equal(isPlainObject([]), false);
  assert.equal(isPlainObject(null), false);
  assert.equal(isPlainObject('x'), false);
});

test('merge walks nested options', () => {
  assert.deepEqual(merge({ a: { b: 1 } }, { a: { c: 2 } }), { a: { b: 1, c: 2 } });
  assert.deepEqual(merge({ a: 1 }, { b: 2 }), { a: 1, b: 2 });
});

test('merge copies nested options instead of sharing them', () => {
  const patch = { headers: { accept: 'text/html' } };
  const merged = merge({}, patch);
  merged.headers.accept = 'application/json';
  assert.equal(patch.headers.accept, 'text/html');
});

test('applyEnvelope merges over the defaults without mutating them', () => {
  const defaults = { timeoutMs: 1000, retries: 3, headers: { accept: 'text/html' } };
  const options = applyEnvelope('{"timeoutMs":500,"headers":{"x-plugin":"1"}}', defaults);
  assert.deepEqual(options, {
    timeoutMs: 500,
    retries: 3,
    headers: { accept: 'text/html', 'x-plugin': '1' },
  });
  assert.deepEqual(defaults, { timeoutMs: 1000, retries: 3, headers: { accept: 'text/html' } });
});

test('pick keeps the options this plugin understands', () => {
  assert.deepEqual(pick({ timeoutMs: 5, retries: 2, secret: 'x' }, ['timeoutMs', 'retries']), {
    timeoutMs: 5,
    retries: 2,
  });
  assert.deepEqual(pick({ timeoutMs: 5, retries: 2 }), { timeoutMs: 5, retries: 2 });
});

test('coerceNumbers converts the named options', () => {
  assert.deepEqual(coerceNumbers({ timeoutMs: '250', retries: '2' }, ['timeoutMs', 'retries']), {
    timeoutMs: 250,
    retries: 2,
  });
  assert.throws(() => coerceNumbers({ timeoutMs: 'soon' }, ['timeoutMs']), EnvelopeError);
  assert.throws(() => coerceNumbers({ timeoutMs: -1 }, ['timeoutMs']), EnvelopeError);
});

test('headerMap lowercases the names', () => {
  assert.deepEqual(headerMap(['Accept: text/html', 'X-Plugin: v2']), {
    accept: 'text/html',
    'x-plugin': 'v2',
  });
});

test('headerMap trims the value', () => {
  assert.deepEqual(headerMap(['Accept:   text/html   ']), { accept: 'text/html' });
});

test('headerMap refuses a malformed line', () => {
  assert.throws(() => headerMap(['no separator']), EnvelopeError);
  assert.throws(() => headerMap([': value']), EnvelopeError);
  assert.throws(() => headerMap('Accept: text/html'), EnvelopeError);
});
