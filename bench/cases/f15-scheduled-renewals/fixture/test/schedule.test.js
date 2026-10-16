import test from 'node:test';
import assert from 'node:assert/strict';
import { RenewalCalendar } from '../src/calendar.js';
import { RetrySchedule } from '../src/backoff.js';

test('monthly renewals use UTC and cross the year boundary', () => {
  const calendar = new RenewalCalendar('2025-12-15');
  assert.equal(calendar.advance(), '2026-01-15');
  assert.equal(calendar.advance(), '2026-02-15');
});

test('renewals fit within the destination month', () => {
  assert.equal(new RenewalCalendar('2024-01-31').advance(), '2024-02-29');
});

test('retry offsets accumulate and respect the configured cap', () => {
  const schedule = new RetrySchedule(10, 100);
  assert.equal(schedule.next(), 10);
  assert.equal(schedule.next(), 30);
  const capped = new RetrySchedule(10, 15);
  assert.equal(capped.next(), 10);
  assert.equal(capped.next(), 25);
});
