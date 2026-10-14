import assert from 'node:assert/strict';
import test from 'node:test';
import { monthlyRenewal, annualRenewal } from '../src/renewal.js';

test('monthly subscription dates', () => {
  assert.equal(monthlyRenewal('2024-05-15'), '2024-06-15');
  assert.equal(monthlyRenewal('2024-12-10'), '2025-01-10');
});

test('annual subscription dates', () => {
  assert.equal(annualRenewal('2024-05-15'), '2025-05-15');
  assert.equal(annualRenewal('2023-02-20'), '2024-02-20');
});
