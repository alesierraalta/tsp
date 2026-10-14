import test from 'node:test';
import assert from 'node:assert/strict';
import { Sessions } from '../src/index.js';

test('session delivery and renewal', () => {
  const sessions = new Sessions(() => 100);
  const token = sessions.issue('alice', 50);
  const renewed = sessions.refresh(token, 100);
  const ticket = sessions.submit(renewed, 'hello');
  sessions.dispatch(ticket);
  assert.deepEqual(sessions.effects(), [{ subject: 'alice', message: 'hello' }]);
  assert.throws(() => sessions.dispatch(ticket));
  const effects = sessions.effects();
  effects[0].message = 'local';
  assert.equal(sessions.effects()[0].message, 'hello');
});

test('session admission', () => {
  let now = 0;
  const sessions = new Sessions(() => now);
  const token = sessions.issue('alice', 10);
  now = 10;
  assert.throws(() => sessions.submit(token, 'hello'));
  assert.throws(() => sessions.refresh(token, 10));
  const other = sessions.issue('bob', 10);
  sessions.revoke(other);
  assert.throws(() => sessions.submit(other, 'hello'));
  assert.throws(() => sessions.submit('unknown', 'hello'));
  assert.throws(() => sessions.issue('alice', 0));
});

test('queue handles', () => {
  const sessions = new Sessions(() => 0);
  const token = sessions.issue('alice', 10);
  const ticket = sessions.submit(token, 'hello');
  sessions.revoke(token);
  assert.throws(() => sessions.dispatch(ticket));
  assert.throws(() => sessions.dispatch(Symbol()));
  assert.deepEqual(sessions.effects(), []);
});
