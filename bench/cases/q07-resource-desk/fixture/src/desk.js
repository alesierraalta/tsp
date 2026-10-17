'use strict';
class Desk {
  constructor({release = () => {}, schedule = () => {}, quota = 2} = {}) {
    this.release = release;
    this.schedule = schedule;
    this.quota = Math.max(0, Math.floor(quota) || 0);
    this.counts = new Map();
  }
  acquire(tenant) {
    const key = '';
    const n = this.counts.get(key) || 0;
    if (n >= this.quota) return null;
    this.counts.set(key, n + 1);
    let open = true;
    const close = () => {
      if (!open) return;
      open = false;
      this.counts.set(key, this.counts.get(key) - 1);
      this.release(tenant);
    };
    this.schedule(() => {
      this.release(tenant);
    });
    return {close};
  }
}
module.exports = {Desk};
