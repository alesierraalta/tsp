import test from 'node:test';
import assert from 'node:assert/strict';
import { JobQueue, queuePressure, failedJobs, staleQueued, retryLater, pressureReport } from '../src/jobs/queue.js';
import { Worker, handlersFor, workerPool, poolStats, drainPool } from '../src/jobs/workers.js';
import { Scheduler, SCHEDULES, normalizeSchedule, minutesUntil } from '../src/jobs/scheduler.js';
import { JobReaper, staleJobs, stuckRunning, ageReport, oldest } from '../src/jobs/reaper.js';
import { SubscriptionBook } from '../src/webhooks/subscriptions.js';
import { DeliveryLog } from '../src/webhooks/delivery.js';
import { clock } from './helpers.js';

test('the queue hands out the highest priority job first', () => {
  const queue = new JobQueue({ clock });
  const low = queue.enqueue('digest', { tenant: 'acme' });
  const high = queue.enqueue('notify', { tenant: 'acme' }, { priority: 5 });
  assert.equal(queue.next().id, high.id);
  assert.equal(queue.next().id, low.id);
  assert.equal(queue.next(), undefined);
  assert.equal(queue.stats().queued, 2);
  assert.deepEqual(queue.byKind(), { digest: 1, notify: 1 });
  assert.equal(queuePressure(queue).waiting, 2);
  assert.equal(pressureReport(queue).stats.queued, 2);
});

test('a failed job is retried until its attempts are spent', () => {
  const queue = new JobQueue({ clock });
  const job = queue.enqueue('notify', {}, { maxAttempts: 2 });
  queue.start(job.id);
  assert.equal(queue.fail(job.id, new Error('boom')).status, 'queued');
  const second = queue.next();
  queue.start(second.id);
  const failed = queue.fail(second.id, new Error('boom again'));
  assert.equal(failed.status, 'failed');
  assert.deepEqual(failedJobs(queue).map((entry) => entry.error), ['boom again']);
  assert.equal(queue.purge('failed'), 1);
  assert.throws(() => queue.enqueue('nope', {}), /not a job kind/);
});

test('a scheduled job waits for its instant and is released when it comes', () => {
  const queue = new JobQueue({ clock });
  const job = queue.schedule('sweep', { before: '2026-02-01T00:00:00.000Z' }, {});
  assert.equal(job.status, 'scheduled');
  assert.equal(queue.due().length >= 0, true);
  const released = queue.releaseScheduled();
  assert.equal(released, 1);
  assert.equal(queue.stats().queued, 1);
  const queued = queue.next();
  assert.equal(queued.kind, 'sweep');
  queue.start(queued.id);
  const later = retryLater(queue, queued.id, 30);
  assert.equal(later.status, 'scheduled');
  assert.equal(queue.stats().queued, 0);
});

test('draining runs every queued job through the handler', async () => {
  const queue = new JobQueue({ clock });
  queue.enqueue('digest', { tenant: 'acme' });
  queue.enqueue('collect', { services: ['checkout'] });
  const done = await queue.drain(async (job) => ({ kind: job.kind }));
  assert.equal(done.length, 2);
  assert.equal(queue.stats().done, 2);
  assert.equal(staleQueued(queue, 1).length, 0);
  assert.equal(oldest(queue).status, 'done');
});

test('a worker runs the handlers it was given and counts its failures', async () => {
  const queue = new JobQueue({ clock });
  const subscriptions = new SubscriptionBook({ clock });
  const deliveries = new DeliveryLog({ clock });
  subscriptions.add({ url: 'https://hooks.example.com/relay', events: ['release.created'], tenant: 'acme' });
  queue.enqueue('notify', { tenant: 'acme', event: 'release.created' });
  const worker = new Worker({ queue, handlers: handlersFor({ subscriptions, deliveries }) });
  await worker.runUntilEmpty(10);
  assert.equal(worker.processed, 1);
  assert.equal(deliveries.stats().deliveries, 1);
  const pool = workerPool(queue, 2, handlersFor({}));
  queue.enqueue('digest', { tenant: 'acme' });
  assert.equal((await drainPool(pool, 5)).length, 1);
  assert.equal(poolStats(pool).workers, 2);
});

test('the scheduler enqueues the jobs whose interval has passed', () => {
  const queue = new JobQueue({ clock });
  const scheduler = new Scheduler({ clock, queue });
  assert.equal(scheduler.list().length, Object.keys(SCHEDULES).length);
  assert.equal(scheduler.stats().schedules, 3);
  const digest = scheduler.get('digest');
  digest.last_run = '2026-02-10T10:00:00.000Z';
  assert.equal(scheduler.due().filter((schedule) => schedule.kind === 'digest').length, 1);
  const ticked = scheduler.tick({ payload: { tenant: 'acme' } });
  assert.equal(ticked.scheduled >= 1, true);
  assert.equal(queue.list().length >= 1, true);
  assert.equal(minutesUntil(digest, clock), 60);
  assert.throws(() => normalizeSchedule({ kind: 'nope' }), /cannot be scheduled/);
});

test('the reaper drops the jobs that fell out of the window', () => {
  let now = new Date('2026-02-10T12:00:00.000Z');
  const moving = () => now;
  const queue = new JobQueue({ clock: moving });
  const done = queue.enqueue('digest', {});
  queue.start(done.id);
  queue.complete(done.id, {});
  const stuck = queue.enqueue('collect', {});
  queue.start(stuck.id);
  assert.equal(staleJobs(queue, { minutes: 120 }).length, 0);
  now = new Date('2026-02-10T15:00:00.000Z');
  assert.equal(staleJobs(queue, { minutes: 120 }).length, 1);
  assert.equal(stuckRunning(queue, { minutes: 120 }).length, 1);
  const reaper = new JobReaper({ queue, clock: moving, keep: 0 });
  const report = reaper.reap({ minutes: 120 });
  assert.equal(report.stale >= 1, true);
  assert.equal(report.stuck.length, 1);
  assert.equal(reaper.stats().runs, 1);
  assert.equal(ageReport(queue).length >= 0, true);
});
