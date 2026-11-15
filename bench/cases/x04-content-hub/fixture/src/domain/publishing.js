import { conflict } from '../util/errors.js';
import { isPast, toIso } from '../util/time.js';
import { validateEntry } from './validation.js';

export const TRANSITIONS = {
  draft: ['published', 'archived'],
  published: ['draft', 'archived'],
  archived: ['draft'],
};

export function canTransition(from, to) {
  return (TRANSITIONS[from] ?? []).includes(to);
}

export function assertTransition(from, to) {
  if (!canTransition(from, to)) {
    throw conflict('invalid_transition', `a ${from} entry cannot become ${to}`);
  }
  return true;
}

export class PublishWorkflow {
  constructor(options = {}) {
    this.entries = options.entries;
    this.clock = options.clock ?? (() => new Date());
  }

  publish(schema, id, options = {}) {
    const entry = this.entries.get(id);
    assertTransition(entry.status, 'published');
    if (schema.draft_and_publish && !options.force) {
      const check = validateEntry(schema, entry.values, {});
      if (!check.ok) {
        const problem = new Error(`entry ${entry.slug} is not publishable: ${check.errors[0].message}`);
        problem.code = 'unpublishable';
        problem.status = 409;
        problem.details = { errors: check.errors };
        problem.toJSON = () => ({ error: 'unpublishable', message: problem.message, details: problem.details });
        throw problem;
      }
    }
    return this.entries.setStatus(id, 'published', options);
  }

  unpublish(id, options = {}) {
    const entry = this.entries.get(id);
    assertTransition(entry.status, 'draft');
    return this.entries.setStatus(id, 'draft', options);
  }

  archive(id, options = {}) {
    const entry = this.entries.get(id);
    assertTransition(entry.status, 'archived');
    return this.entries.setStatus(id, 'archived', options);
  }

  dueForPublish(records) {
    return records.filter((record) => record.status === 'draft' && record.values?.publish_at && isPast(record.values.publish_at, this.clock));
  }

  schedule(schema, id, when, options = {}) {
    const entry = this.entries.get(id);
    const at = toIso(when);
    if (!at) throw new TypeError('publish_at must be a date');
    const values = { ...entry.values, publish_at: at };
    return this.entries.update(id, schema, { values }, options);
  }

  history(records) {
    const out = { draft: 0, published: 0, archived: 0 };
    for (const record of records) out[record.status] = (out[record.status] ?? 0) + 1;
    return out;
  }

  publishedSince(records, instant) {
    const cutoff = new Date(instant).getTime();
    return records.filter((record) => record.published_at && new Date(record.published_at).getTime() >= cutoff);
  }
}

export function publishSummary(records) {
  const published = records.filter((record) => record.status === 'published');
  const drafts = records.filter((record) => record.status === 'draft');
  return {
    total: records.length,
    published: published.length,
    drafts: drafts.length,
    latest: published
      .map((record) => record.published_at)
      .filter(Boolean)
      .sort()
      .pop() ?? null,
  };
}

export function statusTimeline(records) {
  return records
    .flatMap((record) =>
      (record.history ?? []).map((step) => ({ id: record.id, slug: record.slug, ...step })),
    )
    .sort((left, right) => (left.at < right.at ? -1 : 1));
}

export function staleDrafts(records, clock = () => new Date(), days = 30) {
  const cutoff = new Date(clock()).getTime() - days * 24 * 60 * 60 * 1000;
  return records.filter(
    (record) => record.status === 'draft' && new Date(record.updated_at ?? record.created_at).getTime() < cutoff,
  );
}

export function nextPublishSlot(records, instant) {
  const when = new Date(instant).getTime();
  const scheduled = records
    .filter((record) => record.status === 'draft' && record.values?.publish_at)
    .map((record) => ({ id: record.id, at: new Date(record.values.publish_at).getTime() }))
    .filter((entry) => entry.at > when)
    .sort((left, right) => left.at - right.at);
  return scheduled[0];
}

export function publishRate(records, days = 7, clock = () => new Date()) {
  const cutoff = new Date(clock()).getTime() - days * 24 * 60 * 60 * 1000;
  const recent = records.filter(
    (record) => record.published_at && new Date(record.published_at).getTime() >= cutoff,
  );
  return days === 0 ? 0 : Math.round((recent.length / days) * 100) / 100;
}
