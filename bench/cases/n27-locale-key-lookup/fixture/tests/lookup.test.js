import assert from 'node:assert/strict';
import test from 'node:test';
import { createLookup } from '../src/lookup.js';

test('matches ordinary keys without regard to ASCII case', () => {
  const lookup = createLookup({ alpha: 'first', BETA: 'second' });
  assert.equal(lookup.get('ALPHA'), 'first');
  assert.equal(lookup.get('beta'), 'second');
});

test('retains distinct plain accented and unaccented keys', () => {
  const lookup = createLookup({ café: 'accented', cafe: 'plain' });
  assert.equal(lookup.get('CAFÉ'), 'accented');
  assert.equal(lookup.get('CAFE'), 'plain');
});
