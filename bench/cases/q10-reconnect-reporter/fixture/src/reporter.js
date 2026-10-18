class Reporter {
  constructor({ sink = { send: () => true }, pool = { acquire: () => ({}), release: () => {} }, limit = 4 } = {}) {
    this.sink = sink;
    this.pool = pool;
    this.limit = Math.max(0, Math.floor(Number.isFinite(limit) ? limit : 0));
    this.current = null;
    this.pending = new Map();
    this.online = true;
    this.serial = 0;
  }
  report(value) {
    const handle = this.pool.acquire(value);
    this.current = handle;
    const ok = this.sink.send(value);
    if (ok) {
      this.pool.release(this.current);
      this.current = null;
    }
    return !!ok;
  }
  disconnect() { this.online = false; }
  connect(id) {
    this.online = true;
    this.serial++;
    if (this.limit > 0) this.pending.set(this.serial, id);
  }
  drain() {
    if (!this.online) return [];
    const items = [...this.pending.values()];
    this.pending.clear();
    return items;
  }
}
module.exports = { Reporter };
