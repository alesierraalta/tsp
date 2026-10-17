import { createHash } from 'node:crypto';

export class Vault {
  constructor({ fingerprint = text => createHash('sha256').update(text).digest('hex'), evaluator } = {}) {
    this.fingerprint = fingerprint;
    this.evaluator = evaluator ?? (async text => ({ status: 0, output: String(text.trim().split(/\s+/).filter(Boolean).length) }));
    this.documents = new Map();
    this.index = new Map();
    this.sequence = 0;
  }

  put(content) {
    const key = JSON.stringify([this.fingerprint(content), content]);
    const prior = this.index.get(key);
    if (prior !== undefined) return prior;
    const id = String(++this.sequence);
    this.documents.set(id, { content, score: null });
    this.index.set(key, id);
    return id;
  }

  get(id) {
    const row = this.documents.get(id);
    return row === undefined ? undefined : { ...row };
  }

  async evaluate(id) {
    const row = this.documents.get(id);
    if (row === undefined) return false;
    const result = await this.evaluator(row.content);
    if (result.status !== 0) return false;
    const score = Number.parseFloat(result.output);
    if (!Number.isFinite(score)) return false;
    this.documents.set(id, { ...row, score });
    return true;
  }
}
