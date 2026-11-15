import { newId } from '../util/ids.js';
import { nowIso } from '../util/time.js';

export class JobQueue {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.jobs = new Map();
    this.pending = [];
  }

  enqueue(kind, payload = {}, options = {}) {
    const job = {
      id: newId('job'),
      kind,
      payload,
      priority: options.priority ?? 0,
      attempts: 0,
      max_attempts: options.maxAttempts ?? 3,
      status: 'queued',
      created_at: new Date(this.clock()).toISOString(),
      error: null,
    };
    this.jobs.set(job.id, job);
    this.pending.push(job.id);
    this.sort();
    return job;
  }

  sort() {
    this.pending.sort((left, right) => {
      const a = this.jobs.get(left);
      const b = this.jobs.get(right);
      if (a.priority !== b.priority) return b.priority - a.priority;
      return a.created_at < b.created_at ? -1 : 1;
    });
  }

  next() {
    while (this.pending.length > 0) {
      const job = this.jobs.get(this.pending.shift());
      if (job && job.status === 'queued') return job;
    }
    return undefined;
  }

  start(id) {
    const job = this.jobs.get(id);
    if (!job) return undefined;
    job.status = 'running';
    job.attempts += 1;
    return job;
  }

  complete(id, result) {
    const job = this.jobs.get(id);
    if (!job) return undefined;
    job.status = 'done';
    job.result = result;
    return job;
  }

  fail(id, error) {
    const job = this.jobs.get(id);
    if (!job) return undefined;
    job.error = String(error?.message ?? error);
    if (job.attempts < job.max_attempts) {
      job.status = 'queued';
      this.pending.push(id);
      this.sort();
    } else {
      job.status = 'failed';
    }
    return job;
  }

  list(status) {
    return [...this.jobs.values()].filter((job) => status === undefined || job.status === status);
  }

  stats() {
    const out = { queued: 0, running: 0, done: 0, failed: 0 };
    for (const job of this.jobs.values()) out[job.status] = (out[job.status] ?? 0) + 1;
    return out;
  }

  async drain(handler, options = {}) {
    const limit = options.limit ?? 50;
    const done = [];
    for (let index = 0; index < limit; index += 1) {
      const job = this.next();
      if (!job) break;
      this.start(job.id);
      try {
        done.push(this.complete(job.id, await handler(job)));
      } catch (error) {
        done.push(this.fail(job.id, error));
      }
    }
    return done;
  }

  // A scheduled job is one the sweeper may take later: it keeps the time it was set for, and next() skips it
  // until that time has come, so a nightly sweep is listed without being run.
  schedule(kind, payload = {}, options = {}) {
    const job = this.enqueue(kind, payload, options);
    job.run_at = new Date(new Date(this.clock()).getTime() + Math.max(0, Math.trunc(options.delayMs ?? 0))).toISOString();
    job.status = 'scheduled';
    return job;
  }

  due(kind) {
    const now = new Date(this.clock()).getTime();
    return this.list('scheduled').filter(
      (job) => (kind === undefined || job.kind === kind) && new Date(job.run_at ?? this.clock()).getTime() <= now,
    );
  }

  releaseScheduled() {
    const now = new Date(this.clock()).getTime();
    let released = 0;
    for (const job of this.list('scheduled')) {
      if (new Date(job.run_at ?? this.clock()).getTime() > now) continue;
      job.status = 'queued';
      this.pending.push(job.id);
      released += 1;
    }
    this.sort();
    return released;
  }

  byKind() {
    const out = {};
    for (const job of this.jobs.values()) out[job.kind] = (out[job.kind] ?? 0) + 1;
    return out;
  }

  purge(status) {
    let removed = 0;
    for (const [id, job] of [...this.jobs.entries()]) {
      if (status !== undefined && job.status !== status) continue;
      this.jobs.delete(id);
      this.pending = this.pending.filter((candidate) => candidate !== id);
      removed += 1;
    }
    return removed;
  }

  lastRun() {
    return nowIso(this.clock);
  }
}

export function jobSummary(job) {
  return { id: job.id, kind: job.kind, status: job.status, attempts: job.attempts, error: job.error };
}

export function queuePressure(queue) {
  const stats = queue.stats();
  const waiting = (stats.queued ?? 0) + (stats.scheduled ?? 0);
  return {
    ...stats,
    waiting,
    retrying: queue.list('queued').filter((job) => job.attempts > 0).length,
  };
}

export function failedJobs(queue, limit = 10) {
  return queue
    .list('failed')
    .slice(-limit)
    .map((job) => ({ ...jobSummary(job), kind: job.kind }));
}
