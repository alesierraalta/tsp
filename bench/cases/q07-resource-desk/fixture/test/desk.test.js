'use strict';
const {test} = require('node:test');
const assert = require('node:assert/strict');
const {Desk} = require('../src/desk');
test('capacity is restored by closing a handle', () => {
  const desk = new Desk();
  const first = desk.acquire('alpha');
  const second = desk.acquire('alpha');
  assert.equal(desk.acquire('alpha'), null);
  first.close();
  first.close();
  assert.ok(desk.acquire('alpha'));
  second.close();
});
test('a fresh desk supplies a handle', () => {
  assert.ok(new Desk().acquire('beta'));
});
