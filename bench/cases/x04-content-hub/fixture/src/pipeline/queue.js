import { newId } from '../util/ids.js';

export class JobQueue {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.jobs = new Map();
    this.pending = [];
    this.running = new Map();
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
      started_at: null,
      finished_at: null,
      error: null,
    };
    this.jobs.set(job.id, job);
    this.pending.push(job.id);
    this.sortPending();
    return job;
  }

  sortPending() {
    this.pending.sort((left, right) => {
      const a = this.jobs.get(left);
      const b = this.jobs.get(right);
      if (a.priority !== b.priority) return b.priority - a.priority;
      return a.created_at < b.created_at ? -1 : 1;
    });
  }

  next() {
    while (this.pending.length > 0) {
      const id = this.pending.shift();
      const job = this.jobs.get(id);
      if (job && job.status === 'queued') return job;
    }
    return undefined;
  }

  start(id) {
    const job = this.jobs.get(id);
    if (!job) return undefined;
    job.status = 'running';
    job.attempts += 1;
    job.started_at = new Date(this.clock()).toISOString();
    this.running.set(id, job);
    return job;
  }

  complete(id, result) {
    const job = this.jobs.get(id);
    if (!job) return undefined;
    job.status = 'done';
    job.result = result;
    job.finished_at = new Date(this.clock()).toISOString();
    this.running.delete(id);
    return job;
  }

  fail(id, error) {
    const job = this.jobs.get(id);
    if (!job) return undefined;
    job.error = String(error?.message ?? error);
    this.running.delete(id);
    if (job.attempts < job.max_attempts) {
      job.status = 'queued';
      this.pending.push(id);
      this.sortPending();
    } else {
      job.status = 'failed';
      job.finished_at = new Date(this.clock()).toISOString();
    }
    return job;
  }

  cancel(id) {
    const job = this.jobs.get(id);
    if (!job) return false;
    if (job.status === 'done' || job.status === 'failed') return false;
    job.status = 'cancelled';
    job.finished_at = new Date(this.clock()).toISOString();
    this.pending = this.pending.filter((entry) => entry !== id);
    this.running.delete(id);
    return true;
  }

  get(id) {
    return this.jobs.get(id);
  }

  list(status) {
    const all = [...this.jobs.values()];
    return status === undefined ? all : all.filter((job) => job.status === status);
  }

  stats() {
    const out = { queued: 0, running: 0, done: 0, failed: 0, cancelled: 0 };
    for (const job of this.jobs.values()) out[job.status] = (out[job.status] ?? 0) + 1;
    return out;
  }

  async drain(handler, options = {}) {
    const limit = options.limit ?? 100;
    const done = [];
    for (let index = 0; index < limit; index += 1) {
      const job = this.next();
      if (!job) break;
      this.start(job.id);
      try {
        const result = await handler(job);
        done.push(this.complete(job.id, result));
      } catch (error) {
        done.push(this.fail(job.id, error));
      }
    }
    return done;
  }
}
