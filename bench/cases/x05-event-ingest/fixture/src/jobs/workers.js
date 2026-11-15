export const WORKER_KINDS = ['rollup', 'invoice', 'retention', 'webhook', 'partner_export'];

export class Worker {
  constructor(options = {}) {
    this.kind = options.kind ?? 'rollup';
    this.queue = options.queue;
    this.handlers = options.handlers ?? {};
    this.processed = 0;
    this.failed = 0;
  }

  async runOnce() {
    const job = this.queue.next();
    if (!job) return undefined;
    this.queue.start(job.id);
    const handler = this.handlers[job.kind];
    try {
      if (typeof handler !== 'function') throw new TypeError(`no handler for ${job.kind}`);
      const result = await handler(job);
      this.processed += 1;
      return this.queue.complete(job.id, result);
    } catch (error) {
      this.failed += 1;
      return this.queue.fail(job.id, error);
    }
  }

  async runUntilEmpty(limit = 50) {
    const out = [];
    for (let index = 0; index < limit; index += 1) {
      const done = await this.runOnce();
      if (!done) break;
      out.push(done);
    }
    return out;
  }

  stats() {
    return { kind: this.kind, processed: this.processed, failed: this.failed };
  }
}

export function handlersFor(deps = {}) {
  return {
    rollup: async (job) => ({ rolled: (job.payload.events ?? 0) }),
    invoice: async (job) => ({ invoiced: job.payload.tenant ?? 'all' }),
    retention: async (job) => ({ swept: deps.retention?.sweep(job.payload.tenant) ?? { removed: 0 } }),
    webhook: async (job) => ({ delivered: (job.payload.targets ?? []).length }),
    partner_export: async (job) => ({
      exported: deps.partners?.export(job.payload.partner, job.payload.events ?? [], { format: job.payload.format })?.manifest.rows ?? 0,
    }),
  };
}

export function workerPool(queue, count, handlers) {
  return Array.from({ length: Math.max(1, Math.trunc(count)) }, (_, index) =>
    new Worker({ kind: `worker-${index + 1}`, queue, handlers }),
  );
}

export function poolStats(pool) {
  return {
    workers: pool.length,
    processed: pool.reduce((total, worker) => total + worker.processed, 0),
    failed: pool.reduce((total, worker) => total + worker.failed, 0),
  };
}

export async function drainPool(pool, limit = 20) {
  const out = [];
  for (const worker of pool) out.push(...(await worker.runUntilEmpty(limit)));
  return out.filter(Boolean);
}
