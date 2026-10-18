const test = require('node:test');
const assert = require('node:assert/strict');
const { Reporter } = require('../src/reporter');
test('successful reports', () => {
  const r = new Reporter();
  assert.equal(r.report('first'), true);
  assert.equal(r.report('second'), true);
});
test('empty recovery', () => {
  const r = new Reporter();
  assert.deepEqual(r.drain(), []);
});
