import test from 'node:test';
import assert from 'node:assert/strict';
import { Vault } from '../src/vault.js';

test('stores, evaluates and reuses identical documents', async () => {
  const vault = new Vault();
  const a = vault.put('the river runs');
  const b = vault.put('a quiet hill');
  assert.notEqual(a, b);
  assert.equal(vault.put('the river runs'), a);
  assert.equal(await vault.evaluate(a), true);
  assert.deepEqual(vault.get(a), { content: 'the river runs', score: 3 });
  assert.deepEqual(vault.get(b), { content: 'a quiet hill', score: null });
  const copy = vault.get(a);
  copy.score = 9;
  assert.equal(vault.get(a).score, 3);
  assert.equal(await vault.evaluate('absent'), false);
  assert.equal(vault.get('absent'), undefined);
});
