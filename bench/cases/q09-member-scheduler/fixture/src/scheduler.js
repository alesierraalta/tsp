class Scheduler {
  constructor({ members = [], client = { check: () => 'ok' }, store = new Map() } = {}) {
    this.members = new Set(members);
    this.client = client;
    this.store = store;
    this.waiting = [];
    this.ready = [];
    this.changed = false;
  }
  remove(id) {
    this.changed = this.members.delete(id) || this.changed;
  }
  inspect() {
    for (const id of this.members) {
      const result = this.client.check(id);
      if (this.changed && result !== 'ok') this.members.delete(id);
    }
    this.changed = false;
    this.store.set('members', [...this.members]);
    return [...this.members];
  }
  stage(items) { for (const item of items) this.waiting.push(item); }
  promote() {
    for (const item of this.waiting) this.ready.push(item);
    this.waiting = [];
  }
  tick() {
    const n = this.ready.length > this.waiting.length ? 0 : this.ready.length;
    return this.ready.splice(0, n);
  }
}
module.exports = { Scheduler };
