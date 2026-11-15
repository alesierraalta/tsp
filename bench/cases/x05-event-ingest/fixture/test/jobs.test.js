import test from 'node:test';
import assert from 'node:assert/strict';
import { JobQueue, queuePressure, failedJobs, jobSummary } from '../src/jobs/queue.js';
import { Worker, handlersFor, workerPool, poolStats, drainPool, WORKER_KINDS } from '../src/jobs/workers.js';
import { RetentionWorker, SWEEP_STEPS, sweepWindow, expiredRows, sweepSummary, retentionQueueJob } from '../src/jobs/retention.js';
import { MemoryStore } from '../src/store/memory.js';
import { PartitionedStore } from '../src/store/partitions.js';
import { SeriesStore } from '../src/store/series.js';
import { SnapshotBook } from '../src/store/snapshots.js';
import { AuditLog } from '../src/store/audit.js';
import { PlanBook } from '../src/quota/plans.js';
import { clock } from './helpers.js';

function retentionFixture() {
  const plans = new PlanBook();
  plans.assign('acme', 'growth');
  const store = new MemoryStore({ clock });
  store.insert('events', 'evt_old', { id: 'evt_old', tenant: 'acme', at: '2026-01-01T00:00:00.000Z' });
  store.insert('events', 'evt_new', { id: 'evt_new', tenant: 'acme', at: '2026-02-09T00:00:00.000Z' });
  const partitions = new PartitionedStore({ clock });
  partitions.add('acme', { id: 'evt_old', at: '2026-01-01T00:00:00.000Z' });
  partitions.add('acme', { id: 'evt_new', at: '2026-02-09T00:00:00.000Z' });
  const series = new SeriesStore({ clock });
  series.add('acme', 'page_view', '2026-01-02T10:00:00.000Z', 1);
  series.add('acme', 'page_view', '2026-02-09T10:00:00.000Z', 1);
  const snapshots = new SnapshotBook({ clock });
  snapshots.take('old', { events: 1 });
  const audit = new AuditLog({ clock });
  const queue = new JobQueue({ clock });
  const worker = new RetentionWorker({ clock, plans, store, partitions, series, snapshots, audit, queue });
  return { plans, store, partitions, series, snapshots, audit, queue, worker };
}

test('the queue hands out the highest priority job first', () => {
  const queue = new JobQueue({ clock });
  const low = queue.enqueue('rollup', { events: 1 });
  const high = queue.enqueue('invoice', { tenant: 'acme' }, { priority: 5 });
  assert.equal(queue.next().id, high.id);
  assert.equal(queue.next().id, low.id);
  assert.equal(queue.next(), undefined);
  assert.equal(queue.stats().queued, 2);
  assert.deepEqual(queue.byKind(), { rollup: 1, invoice: 1 });
});

test('a failed job is retried until its attempts are spent', () => {
  const queue = new JobQueue({ clock });
  const job = queue.enqueue('rollup', {}, { maxAttempts: 2 });
  queue.start(job.id);
  assert.equal(queue.fail(job.id, new Error('boom')).status, 'queued');
  const second = queue.next();
  assert.equal(second.attempts, 1);
  queue.start(second.id);
  const failed = queue.fail(second.id, new Error('boom again'));
  assert.equal(failed.status, 'failed');
  assert.equal(failed.error, 'boom again');
  assert.equal(queue.stats().failed, 1);
  assert.deepEqual(failedJobs(queue).map((entry) => entry.error), ['boom again']);
  assert.deepEqual(jobSummary(job).kind, 'rollup');
  assert.equal(queue.purge('failed'), 1);
});

test('draining runs every queued job through the handler', async () => {
  const queue = new JobQueue({ clock });
  queue.enqueue('rollup', { events: 2 });
  queue.enqueue('rollup', { events: 3 });
  const done = await queue.drain(async (job) => ({ rolled: job.payload.events }));
  assert.equal(done.length, 2);
  assert.deepEqual(done.map((job) => job.result.rolled), [2, 3]);
  assert.equal(queue.stats().done, 2);
  assert.deepEqual(queuePressure(queue).waiting, 0);
});

test('a scheduled job waits for its instant before it is queued', () => {
  const queue = new JobQueue({ clock });
  const later = queue.schedule('retention', { tenant: 'acme' }, { delayMs: 60000 });
  assert.equal(later.status, 'scheduled');
  assert.equal(later.run_at, '2026-02-10T12:01:00.000Z');
  assert.deepEqual(queue.due('retention'), []);
  assert.equal(queue.releaseScheduled(), 0);
  const soon = queue.schedule('retention', { tenant: 'acme' }, { delayMs: 0 });
  assert.deepEqual(queue.due('retention').map((job) => job.id), [soon.id]);
  assert.equal(queue.releaseScheduled(), 1);
  assert.equal(queue.stats().queued, 1);
});

test('a worker drains the queue with the handlers it was given', async () => {
  const queue = new JobQueue({ clock });
  queue.enqueue('rollup', { events: 4 });
  queue.enqueue('unknown', {});
  const worker = new Worker({ queue, handlers: handlersFor({}) });
  await worker.runUntilEmpty(10);
  assert.equal(worker.processed, 1);
  assert.equal(worker.failed, 3);
  assert.equal(queue.stats().failed, 1);
  assert.deepEqual(worker.stats(), { kind: 'rollup', processed: 1, failed: 3 });
  assert.equal(WORKER_KINDS.includes('retention'), true);
  const pool = workerPool(queue, 2, handlersFor({}));
  queue.enqueue('invoice', { tenant: 'acme' });
  assert.equal((await drainPool(pool, 5)).length, 1);
  assert.equal(poolStats(pool).workers, 2);
});

test('the retention window follows the plan the tenant is on', () => {
  assert.deepEqual(sweepWindow('growth', clock), {
    days: 30,
    from: '2026-01-11T12:00:00.000Z',
    cutoff: '2026-01-11T12:00:00.000Z',
  });
  assert.equal(sweepWindow('free', clock).days, 7);
  assert.deepEqual(SWEEP_STEPS, ['events', 'partitions', 'series', 'snapshots']);
  assert.deepEqual(expiredRows([{ at: '2026-01-01T00:00:00.000Z' }, { at: '2026-02-09T00:00:00.000Z' }], '2026-01-11T12:00:00.000Z').length, 1);
});

test('a planned sweep counts what it would remove and leaves it in place', () => {
  const { worker, store, partitions, series } = retentionFixture();
  const planned = worker.plan('acme');
  assert.equal(planned.dry_run, true);
  assert.equal(planned.steps.events.expired, 1);
  assert.equal(planned.steps.partitions.dropped, 0);
  assert.equal(store.count('events'), 2);
  assert.equal(partitions.days('acme').length, 2);
  assert.equal(worker.history('acme').length, 0);
  const applied = worker.sweep('acme');
  assert.equal(applied.dry_run, false);
  assert.equal(applied.steps.events.expired, 1);
  assert.equal(applied.steps.partitions.dropped, 1);
  assert.equal(applied.steps.series.removed, 1);
  assert.equal(store.count('events'), 1);
  assert.equal(partitions.days('acme').length, 1);
  assert.equal(series.stats().points, 1);
  assert.equal(worker.history('acme').length, 1);
  assert.equal(worker.stats().removed >= 3, true);
});

test('a sweep writes one audit row and reports the tenants it covered', () => {
  const { worker, audit, plans } = retentionFixture();
  plans.assign('globex', 'free');
  const all = worker.sweepAll();
  assert.equal(all.runs.length, 1);
  assert.equal(audit.counts()['retention.swept'], 1);
  assert.deepEqual(worker.tenants(), ['acme']);
  assert.deepEqual(sweepSummary(all.runs), { runs: 1, removed: all.removed, byPlan: { growth: all.removed } });
  assert.equal(worker.window('acme').days, 30);
  const job = retentionQueueJob(worker, 'acme');
  assert.equal(job.kind, 'retention');
  assert.equal(job.cutoff, '2026-01-11T12:00:00.000Z');
  assert.equal(job.steps.length, 4);
});

test('a sweep can be scheduled on the queue as a retention job', () => {
  const { worker, queue } = retentionFixture();
  const job = worker.schedule(queue, 'acme', { priority: 2 });
  assert.equal(job.kind, 'retention');
  assert.equal(job.priority, 2);
  assert.equal(queue.list('queued').length, 1);
  assert.deepEqual(worker.sweep('acme', { steps: ['nope'], dry_run: true }).steps, {});
});
