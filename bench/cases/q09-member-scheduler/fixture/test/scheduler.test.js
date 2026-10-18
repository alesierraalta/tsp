const test = require('node:test');
const assert = require('node:assert/strict');
const { Scheduler } = require('../src/scheduler');
test('membership snapshot', () => {
  const s = new Scheduler({members: ['north', 'south']});
  assert.deepEqual(s.inspect(), ['north', 'south']);
  s.remove('south');
  assert.deepEqual(s.inspect(), ['north']);
});
test('empty work', () => {
  const s = new Scheduler();
  assert.deepEqual(s.tick(), []);
});
