import { clone } from '../util/json.js';
import { newId } from '../util/ids.js';

export class RevisionLog {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.limit = options.limit ?? 50;
    this.rows = [];
  }

  record({ collection, id, action, before, after, actor }) {
    const revision = {
      id: newId('rev'),
      collection,
      record_id: id,
      action,
      revision: this.revisionsOf(collection, id).length + 1,
      before: before === undefined ? null : clone(before),
      after: after === undefined ? null : clone(after),
      actor: actor ?? 'system',
      at: new Date(this.clock()).toISOString(),
    };
    this.rows.push(revision);
    this.trim(collection, id);
    return revision;
  }

  revisionsOf(collection, id) {
    return this.rows.filter((row) => row.collection === collection && row.record_id === id);
  }

  trim(collection, id) {
    const own = this.revisionsOf(collection, id);
    if (own.length <= this.limit) return;
    const excess = own.slice(0, own.length - this.limit);
    const drop = new Set(excess.map((row) => row.id));
    this.rows = this.rows.filter((row) => !drop.has(row.id));
  }

  list(collection, id) {
    return this.revisionsOf(collection, id).map((row) => ({
      id: row.id,
      revision: row.revision,
      action: row.action,
      actor: row.actor,
      at: row.at,
    }));
  }

  payload(collection, id, revision) {
    const found = this.revisionsOf(collection, id).find((row) => row.revision === revision);
    if (!found) return undefined;
    return clone(found.after ?? found.before);
  }

  restore(collection, id, revision) {
    const payload = this.payload(collection, id, revision);
    if (payload === undefined) throw new Error(`no revision ${revision} of ${collection} ${id}`);
    return payload;
  }

  recent(limit = 20) {
    return this.rows.slice(-limit).reverse();
  }

  size() {
    return this.rows.length;
  }

  byActor(actor, limit = 20) {
    return this.rows
      .filter((row) => row.actor === actor)
      .slice(-limit)
      .reverse();
  }

  actions() {
    const out = {};
    for (const row of this.rows) out[row.action] = (out[row.action] ?? 0) + 1;
    return out;
  }

  oldest(collection, id) {
    const own = this.revisionsOf(collection, id);
    return own.length === 0 ? undefined : own[0];
  }

  compare(collection, id, from, to) {
    const before = this.payload(collection, id, from);
    const after = this.payload(collection, id, to);
    if (before === undefined || after === undefined) return undefined;
    const changed = {};
    for (const key of new Set([...Object.keys(before), ...Object.keys(after)])) {
      const left = JSON.stringify(before[key] ?? null);
      const right = JSON.stringify(after[key] ?? null);
      if (left !== right) changed[key] = { before: before[key] ?? null, after: after[key] ?? null };
    }
    return changed;
  }

  pruneOlderThan(instant) {
    const cutoff = new Date(instant).getTime();
    const before = this.rows.length;
    this.rows = this.rows.filter((row) => new Date(row.at).getTime() >= cutoff);
    return before - this.rows.length;
  }

  compact() {
    const seen = new Map();
    for (const row of this.rows) {
      const key = `${row.collection}:${row.record_id}`;
      const bucket = seen.get(key) ?? [];
      bucket.push(row);
      seen.set(key, bucket);
    }
    for (const [key, rows] of seen.entries()) {
      if (rows.length <= this.limit) continue;
      const keep = new Set(rows.slice(-this.limit).map((row) => row.id));
      this.rows = this.rows.filter((row) => {
        const own = `${row.collection}:${row.record_id}` !== key;
        return own || keep.has(row.id);
      });
    }
    return this.rows.length;
  }
}
