export class OffsetStore {
  constructor() {
    this.offsets = new Map();
  }

  key(topic, consumer) {
    return `${topic}\u0000${consumer}`;
  }

  read(topic, consumer = 'default') {
    return this.offsets.get(this.key(topic, consumer)) ?? 0;
  }

  advance(topic, consumer, position) {
    const next = Number(position);
    if (!Number.isInteger(next) || next < 0) throw new TypeError('an offset must be a whole number');
    const key = this.key(topic, consumer);
    const current = this.offsets.get(key) ?? 0;
    if (next < current) return current;
    this.offsets.set(key, next);
    return next;
  }

  reset(topic, consumer = 'default') {
    const key = this.key(topic, consumer);
    const current = this.offsets.get(key) ?? 0;
    this.offsets.delete(key);
    return current;
  }

  snapshot() {
    const out = {};
    for (const [key, value] of this.offsets.entries()) {
      const [topic, consumer] = key.split('\u0000');
      out[`${topic}:${consumer}`] = value;
    }
    return out;
  }

  topics() {
    return [...new Set([...this.offsets.keys()].map((key) => key.split('\u0000')[0]))].sort();
  }

  consumers(topic) {
    return [...this.offsets.keys()]
      .filter((key) => key.startsWith(`${topic}\u0000`))
      .map((key) => key.split('\u0000')[1])
      .sort();
  }

  count() {
    return this.offsets.size;
  }
}

export function nextBatch(rows, offset, size = 50) {
  const start = Math.max(0, Number(offset) || 0);
  const limit = Math.max(0, Number(size) || 0);
  return rows.slice(start, start + limit);
}
