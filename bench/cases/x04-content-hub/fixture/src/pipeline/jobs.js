import { notFound } from '../util/errors.js';

export const JOB_KINDS = ['render', 'export', 'reindex', 'transform', 'prune'];

export function jobSpec(kind) {
  const spec = JOB_SPECS[kind];
  if (!spec) throw new TypeError(`unknown job kind ${kind}`);
  return spec;
}

const JOB_SPECS = {
  render: { label: 'render entries', retries: 2, requires: ['entryIds'] },
  export: { label: 'export a content type', retries: 1, requires: ['apiId', 'format'] },
  reindex: { label: 'rebuild the search index', retries: 1, requires: [] },
  transform: { label: 'run a plugin transform', retries: 1, requires: ['plugin', 'entryId'] },
  prune: { label: 'prune old revisions', retries: 0, requires: ['keep'] },
};

export function validateJob(kind, payload = {}) {
  const spec = jobSpec(kind);
  const missing = spec.requires.filter((key) => payload[key] === undefined);
  if (missing.length > 0) throw new TypeError(`${kind} needs ${missing.join(', ')}`);
  return spec;
}

export class JobRunner {
  constructor(options = {}) {
    this.queue = options.queue;
    this.entries = options.entries;
    this.renderer = options.renderer;
    this.exporters = options.exporters;
    this.search = options.search;
    this.plugins = options.plugins;
    this.handlers = new Map();
    for (const kind of JOB_KINDS) this.handlers.set(kind, this.defaultHandler(kind));
    for (const [kind, handler] of Object.entries(options.handlers ?? {})) this.handlers.set(kind, handler);
  }

  submit(kind, payload = {}, options = {}) {
    validateJob(kind, payload);
    return this.queue.enqueue(kind, payload, options);
  }

  handler(kind) {
    const found = this.handlers.get(kind);
    if (!found) throw notFound(`job kind ${kind}`);
    return found;
  }

  defaultHandler(kind) {
    return async (job) => {
      switch (kind) {
        case 'render': {
          const entries = (job.payload.entryIds ?? []).map((id) => this.entries.get(id));
          return { rendered: await this.renderer.renderMany(entries, job.payload.options ?? {}) };
        }
        case 'export': {
          const result = await this.exporters.exportType(job.payload.apiId, job.payload.format, job.payload.options ?? {});
          return { rows: result.rows, name: result.name };
        }
        case 'reindex': {
          const result = this.search.rebuild(this.entries.list({ api_id: job.payload.apiId }));
          return { documents: result };
        }
        case 'transform': {
          const entry = this.entries.get(job.payload.entryId);
          return this.plugins.transform(job.payload.plugin, entry);
        }
        case 'prune': {
          const keep = Number(job.payload.keep ?? 10);
          return { kept: keep, pruned: 0 };
        }
        default:
          throw new TypeError(`no handler for ${kind}`);
      }
    };
  }

  async run(kind, payload, options = {}) {
    const job = this.submit(kind, payload, options);
    const handler = this.handler(kind);
    this.queue.start(job.id);
    try {
      const result = await handler(this.queue.get(job.id));
      return this.queue.complete(job.id, result);
    } catch (error) {
      return this.queue.fail(job.id, error);
    }
  }

  async drain(options = {}) {
    return this.queue.drain((job) => this.handler(job.kind)(job), options);
  }

  stats() {
    return this.queue.stats();
  }
}

export function jobSummary(job) {
  return {
    id: job.id,
    kind: job.kind,
    status: job.status,
    attempts: job.attempts,
    error: job.error,
  };
}
