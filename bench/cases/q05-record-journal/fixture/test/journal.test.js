import test from 'node:test';
import assert from 'node:assert/strict';
import { Journal } from '../src/journal.js';

test('ingests complete records and rejects incomplete records', () => {
  const j = new Journal();
  assert.equal(j.ingest(['a', 'blue']), true);
  assert.deepEqual(j.rows.get('a'), { key: 'a', value: 'blue' });
  assert.equal(j.ingest(['b']), false);
  assert.equal(j.rows.has('b'), false);
});

test('commits independent transactions', () => {
  const j = new Journal();
  const a = j.begin('first');
  const b = j.begin('second');
  assert.notEqual(a, b);
  assert.equal(j.commit(b), 'second');
  assert.equal(j.commit(a), 'first');
  assert.equal(j.commit(a), undefined);
});
