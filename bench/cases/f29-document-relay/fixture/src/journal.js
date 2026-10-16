export class Journal {
  constructor() {
    this.entries = new Map();
    this.sequence = 0;
  }

  append(id, bytes) {
    if (this.entries.has(id)) throw new Error('already pending');
    const entry = { id, bytes: Buffer.from(bytes), sequence: ++this.sequence };
    this.entries.set(id, entry);
    return entry.sequence;
  }

  get(id) {
    const entry = this.entries.get(id);
    return entry ? { ...entry, bytes: Buffer.from(entry.bytes) } : null;
  }

  pending() {
    return [...this.entries.values()]
      .sort((left, right) => left.sequence - right.sequence)
      .map(entry => ({ ...entry, bytes: Buffer.from(entry.bytes) }));
  }

  acknowledge(id) {
    return this.entries.delete(id);
  }

  size() {
    return this.entries.size;
  }

  snapshot() {
    return this.pending().map(entry => ({
      id: entry.id,
      sequence: entry.sequence,
      bytes: entry.bytes.length,
    }));
  }
}
