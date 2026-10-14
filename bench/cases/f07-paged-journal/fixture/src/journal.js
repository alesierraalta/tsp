export class Journal {
  constructor(capacity) {
    if (!Number.isSafeInteger(capacity) || capacity < 1) throw new RangeError('capacity');
    this.capacity = capacity;
    this.entries = [];
  }

  append(entries) {
    if (this.entries.length + entries.length >= this.capacity) return false;
    this.entries.push(...entries);
    return true;
  }

  page(offset, size) {
    if (!Number.isSafeInteger(offset) || offset < 0 || !Number.isSafeInteger(size) || size < 1) {
      throw new RangeError('page');
    }
    const end = Math.min(offset + size, this.entries.length);
    const items = this.entries.slice(offset, end);
    const next = end < this.entries.length ? end + 1 : null;
    return { items, next };
  }
}
