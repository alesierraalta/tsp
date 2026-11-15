import test from 'node:test';
import assert from 'node:assert/strict';
import { PlanBook, PLANS, planRank, isUpgrade } from '../src/quota/plans.js';
import { ReservationBook } from '../src/quota/reserve.js';
import { UsageLedger, usageSummary, overageRows } from '../src/quota/usage.js';
import { Enforcer, enforcementReport, headroom, wouldExceed, rejectionReport } from '../src/quota/enforcement.js';
import { assertWithinLimits, limitsFor, DEFAULT_LIMITS } from '../src/ingest/limits.js';
import { clock } from './helpers.js';

test('a tenant is on a plan and both plans report the same limits', () => {
  const plans = new PlanBook();
  assert.equal(plans.for('acme').name, 'free');
  plans.assign('acme', 'growth');
  assert.equal(plans.for('acme').name, 'growth');
  assert.equal(plans.for('acme').limits.max_samples, 100000);
  assert.throws(() => plans.assign('acme', 'enterprise'), /unknown plan/);
  assert.deepEqual(plans.tenantsOn('growth'), ['acme']);
  assert.equal(plans.stats().assignments, 1);
  assert.equal(plans.list().length, Object.keys(PLANS).length);
  assert.equal(planRank('scale') > planRank('growth'), true);
  assert.equal(isUpgrade('growth', 'free'), false);
  assert.equal(isUpgrade('growth', 'scale'), true);
  assert.deepEqual(limitsFor(plans.for('acme')), { ...DEFAULT_LIMITS, max_rows: 5000, max_samples: 100000, max_bytes: 4 * 1024 * 1024 });
});

test('a request past a plan limit is refused with the code the API returns', () => {
  const plan = { name: 'free', limits: { max_rows: 10, max_samples: 20, max_bytes: 100 } };
  assert.throws(() => assertWithinLimits(plan, { rows: 11 }), (error) => error.code === 'batch_too_large' && error.status === 400);
  assert.throws(() => assertWithinLimits(plan, { samples: 21 }), (error) => error.code === 'batch_too_large');
  assert.throws(() => assertWithinLimits(plan, { bytes: 101 }), (error) => error.code === 'payload_too_large');
  assert.throws(() => assertWithinLimits(plan, { rows: 1.5 }), (error) => error.code === 'invalid_rows');
  assert.throws(() => assertWithinLimits(plan, { samples: -1 }), (error) => error.code === 'invalid_samples');
  assert.equal(assertWithinLimits(plan, { rows: 10, samples: 20, bytes: 100 }).max_rows, 10);
});

test('a reservation inside the plan is accepted and can be released', () => {
  const plans = new PlanBook();
  plans.assign('acme', 'growth');
  const reservations = new ReservationBook({ plans, clock });
  const reservation = reservations.reserve('acme', { rows: 10, samples: 100 });
  assert.equal(reservation.plan, 'growth');
  assert.equal(reservation.released_at, null);
  assert.equal(reservation.slots, 100);
  assert.equal(reservations.usedSlots('acme'), 100);
  assert.equal(reservations.release(reservation.id), true);
  assert.equal(reservations.usedSlots('acme'), 0);
  assert.equal(reservations.list('acme')[0].released_at, '2026-02-10T12:00:00.000Z');
  assert.equal(reservations.get(reservation.id).released_at, '2026-02-10T12:00:00.000Z');
  assert.equal(reservations.stats().reservations, 1);
  assert.equal(reservations.assertCapacity('acme') >= 0, true);
});

test('a reservation past the plan sample cap is refused before it holds anything', () => {
  const plans = new PlanBook();
  plans.assign('acme', 'free');
  const reservations = new ReservationBook({ plans, clock });
  assert.throws(() => reservations.reserve('acme', { rows: 1, samples: 5000 }), /exceed the 2000/);
  assert.equal(reservations.stats().reservations, 0);
  assert.equal(reservations.usedSlots('acme'), 0);
});

test('usage is accumulated per period and reported as a share of the plan', () => {
  const plans = new PlanBook();
  plans.assign('acme', 'free');
  const usage = new UsageLedger({ plans, clock });
  usage.record('acme', 'current', { rows: 1600, samples: 100, bytes: 10, batches: 1 });
  const capacity = usage.capacity('acme');
  assert.equal(capacity.plan, 'free');
  assert.equal(capacity.used.rows, 1600);
  assert.equal(capacity.remaining.rows, 0);
  assert.equal(capacity.percent.samples, 5);
  assert.deepEqual(usage.warnings('acme', 'current', 50), [{ metric: 'rows', percent: 320, threshold: 50 }]);
  assert.deepEqual(usageSummary(usage, 'acme', 'current'), { plan: 'free', batches: 1, rows: 1600, remaining_rows: 0 });
  assert.equal(usage.reset('acme', 'current'), true);
  assert.equal(usage.get('acme', 'current').rows, 0);
  assert.equal(overageRows(plans.for('acme'), { rows: 2500 }), 2000);
  assert.equal(usage.stats().ledgers, 0);
});

test('the enforcer records every refusal with the reason it refused', () => {
  const plans = new PlanBook();
  plans.assign('acme', 'free');
  const usage = new UsageLedger({ plans, clock });
  const enforcer = new Enforcer({ plans, usage, clock });
  assert.deepEqual(enforcer.checkBatch('acme', { rows: 10 }), { plan: 'free', rows: 10, limit: 500 });
  assert.equal(enforcer.checkSamples('acme', 100), 100);
  assert.equal(enforcer.checkBytes('acme', 100), 100);
  assert.throws(() => enforcer.checkBatch('acme', { rows: 501 }), /exceed the 500 rows/);
  assert.throws(() => enforcer.checkSamples('acme', 2001), /exceed the 2000/);
  assert.throws(() => enforcer.checkBytes('acme', 1024 * 1024), /exceed the 262144/);
  assert.equal(enforcer.stats().rejections, 3);
  assert.equal(enforcer.stats().byReason.rows, 1);
  assert.equal(enforcer.recent(1)[0].reason, 'bytes');
  assert.equal(enforcementReport(enforcer, 'acme').plan, 'free');
  assert.deepEqual(rejectionReport(enforcer, 'acme').byReason, { rows: 1, samples: 1, bytes: 1 });
});

test('headroom says what is left before a limit is reached', () => {
  const plan = { name: 'free', limits: { max_rows: 500, max_samples: 2000, max_bytes: 262144 } };
  assert.deepEqual(headroom(plan, { rows: 400, samples: 100, bytes: 1024 }), { plan: 'free', rows: 100, samples: 1900, bytes: 261120 });
  assert.deepEqual(wouldExceed(plan, { rows: 501, samples: 10, bytes: 10 }), { rows: true, samples: false, bytes: false });
});
