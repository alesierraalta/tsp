import { diffMinutes } from '../util/time.js';
import { badRequest } from '../util/errors.js';

export const REAP_KINDS = ['done', 'failed'];

export function staleJobs(queue, options = {}) {
  const minutes = Math.max(1, Math.trunc(options.minutes ?? 120));
  const kinds = (options.kinds ?? REAP_KINDS).filter((kind) => REAP_KINDS.includes(kind));
  return queue.list().filter((job) => kinds.includes(job.status) && queue.ageMinutes(job) >= minutes);
}

export function stuckRunning(queue, options = {}) {
  const minutes = Math.max(1, Math.trunc(options.minutes ?? 30));
  return queue.list('running').filter((job) => queue.ageMinutes(job) >= minutes);
}

export class JobReaper {
  constructor(options = {}) {
    this.queue = options.queue;
    this.clock = options.clock ?? (() => new Date());
    this.keep = options.keep ?? 200;
    this.runs = [];
  }

  reap(options = {}) {
    if (!this.queue) throw badRequest('invalid_reaper', 'a reaper needs a queue');
    const stale = staleJobs(this.queue, options);
    const removals = stale.slice(0, Math.max(0, stale.length - this.keep));
    let removed = 0;
    for (const job of removals) {
      if (this.queue.jobs.delete(job.id)) removed += 1;
      this.queue.pending = this.queue.pending.filter((id) => id !== job.id);
    }
    const stuck = stuckRunning(this.queue, options).map((job) => {
      const failed = this.queue.fail(job.id, new Error('stuck running'));
      return { id: job.id, status: failed?.status ?? 'unknown' };
    });
    const report = { at: new Date(this.clock()).toISOString(), stale: stale.length, removed, stuck };
    this.runs.push(report);
    return report;
  }

  history() {
    return [...this.runs];
  }

  stats() {
    return {
      runs: this.runs.length,
      removed: this.runs.reduce((total, run) => total + run.removed, 0),
      stuck: this.runs.reduce((total, run) => total + run.stuck.length, 0),
    };
  }
}

export function ageReport(queue) {
  const rows = queue.list().map((job) => ({ id: job.id, kind: job.kind, status: job.status, minutes: queue.ageMinutes(job) }));
  return rows.sort((left, right) => right.minutes - left.minutes);
}

export function oldest(queue) {
  return ageReport(queue)[0];
}

export function minutesSince(instant, clock = () => new Date()) {
  return diffMinutes(instant, new Date(clock()));
}
