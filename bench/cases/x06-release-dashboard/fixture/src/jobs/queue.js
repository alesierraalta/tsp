import { badRequest } from '../util/errors.js';
import { newId } from '../util/ids.js';
import { nowIso, addMinutes, diffMinutes } from '../util/time.js';

export const JOB_KINDS = ['notify', 'digest', 'collect', 'sweep'];

export function normalizeJob(kind, payload = {}, options = {}) {
  if (!JOB_KINDS.includes(kind)) throw badRequest('invalid_job', `${kind} is not a job kind`);
  return {
    id: options.id ?? newId('job'),
    kind,
    payload,
    priority: Number(options.priority ?? 0),
    attempts: 0,
    max_attempts: Number(options.maxAttempts ?? 3),
    status: 'queued',
    created_at: nowIso(options.clock),
    run_at: options.delayMs ? addMinutes(options.clock?.() ?? new Date(), options.delayMs / 60000).toISOString() : null,
    error: null,
    result: null,
  };
}

export class JobQueue {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.jobs = new Map();
    this.pending = [];
  }

  enqueue(kind, payload = {}, options = {}) {
    const job = normalizeJob(kind, payload, { clock: this.clock, ...options });
    this.jobs.set(job.id, job);
    this.pending.push(job.id);
    this.sort();
    return job;
  }

  schedule(kind, payload = {}, options = {}) {
    const job = this.enqueue(kind, payload, options);
    job.status = 'scheduled';
    return job;
  }

  sort() {
    this.pending.sort((left, right) => {
      const a = this.jobs.get(left);
      const b = this.jobs.get(right);
      if (a === undefined || b === undefined) return 0;
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

  due() {
    const now = Date.now();
    return [...this.jobs.values()].filter((job) => job.status === 'scheduled' && (job.run_at === null || new Date(job.run_at).getTime() <= now));
  }

  releaseScheduled() {
    let released = 0;
    for (const job of this.due()) {
      job.status = 'queued';
      this.pending.push(job.id);
      released += 1;
    }
    this.sort();
    return released;
  }

  start(id) {
    const job = this.jobs.get(String(id));
    if (!job) return undefined;
    job.status = 'running';
    job.attempts += 1;
    return job;
  }

  complete(id, result) {
    const job = this.jobs.get(String(id));
    if (!job) return undefined;
    job.status = 'done';
    job.result = result ?? null;
    return job;
  }

  fail(id, error) {
    const job = this.jobs.get(String(id));
    if (!job) return undefined;
    job.error = String(error?.message ?? error);
    if (job.attempts < job.max_attempts) {
      job.status = 'queued';
      this.pending.push(job.id);
      this.sort();
    } else {
      job.status = 'failed';
    }
    return job;
  }

  list(status) {
    return [...this.jobs.values()].filter((job) => status === undefined || job.status === status);
  }

  byKind() {
    const out = {};
    for (const job of this.jobs.values()) out[job.kind] = (out[job.kind] ?? 0) + 1;
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

  stats() {
    const out = { queued: 0, running: 0, scheduled: 0, done: 0, failed: 0 };
    for (const job of this.jobs.values()) out[job.status] = (out[job.status] ?? 0) + 1;
    return out;
  }

  ageMinutes(job) {
    return diffMinutes(job.created_at, new Date(this.clock()));
  }
}

export function queuePressure(queue) {
  const stats = queue.stats();
  return {
    ...stats,
    waiting: (stats.queued ?? 0) + (stats.scheduled ?? 0),
    retrying: queue.list('queued').filter((job) => job.attempts > 0).length,
  };
}

export function failedJobs(queue, limit = 10) {
  return queue
    .list('failed')
    .slice(-limit)
    .map((job) => ({ id: job.id, kind: job.kind, error: job.error, attempts: job.attempts }));
}

export function staleQueued(queue, minutes) {
  return queue.list('queued').filter((job) => queue.ageMinutes(job) >= minutes);
}

export function retryLater(queue, id, minutes) {
  const job = queue.jobs.get(String(id));
  if (!job) return undefined;
  job.status = 'scheduled';
  job.run_at = addMinutes(new Date(queue.clock()), Math.max(0, Math.trunc(minutes))).toISOString();
  queue.pending = queue.pending.filter((candidate) => candidate !== job.id);
  return job;
}

export function oldestQueued(queue) {
  return queue.list('queued').reduce((oldest, job) => (oldest === undefined || job.created_at < oldest.created_at ? job : oldest), undefined);
}

export function pressureReport(queue) {
  return {
    stats: queue.stats(),
    byKind: queue.byKind(),
    oldest: oldestQueued(queue)?.id ?? null,
    stale: staleQueued(queue, 120).length,
  };
}
