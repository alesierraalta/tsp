'use strict';
class Channel {
  constructor({client = () => true, store = {read: () => []}, attempts = 2} = {}) {
    this.client = client;
    this.store = store;
    this.attempts = Math.max(1, Math.floor(attempts) || 1);
    this.position = 0;
    this.pending = [];
  }
  enqueue(value) { this.pending.push({value, used: 0}); }
  flush() {
    const next = [];
    for (const item of this.pending) {
      const ok = this.client(item.value);
      item.used++;
      if (!ok && item.used < this.attempts) next.push(item);
    }
    this.pending = next;
  }
  refresh() {
    const entries = this.store.read(0);
    for (const entry of entries) this.position = Math.max(this.position, entry.sequence);
    return entries;
  }
}
module.exports = {Channel};
