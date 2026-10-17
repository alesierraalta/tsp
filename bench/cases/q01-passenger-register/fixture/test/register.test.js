import test from 'node:test';
import assert from 'node:assert/strict';
import { Register } from '../src/register.js';

test('registration and itinerary amendments preserve the profile', () => {
  const register = new Register();
  register.add('a', { name: 'Avery', title: 'Ms', category: 'adult', destination: 'Hill' });
  register.add('b', { name: 'Blair', title: 'Mstr', category: 'child', destination: 'Vale' });
  assert.equal(register.get('a').weight, 84);
  assert.equal(register.get('b').weight, 35);
  assert.equal(register.amend('a', { destination: 'Coast' }), true);
  assert.deepEqual(register.get('a'), { name: 'Avery', title: 'Ms', category: 'adult', destination: 'Coast', weight: 84 });
  assert.equal(register.get('b').destination, 'Vale');
  assert.equal(register.amend('missing', { destination: 'Hill' }), false);
  assert.equal(register.get('missing'), undefined);
});
