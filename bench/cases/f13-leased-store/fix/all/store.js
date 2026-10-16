export class Store {
  constructor() {
    this.values = new Map();
    this.leases = new Map();
    this.listeners = new Map();
  }
  put(key, value) { this.values.set(key, value); }
  get(key) { return this.values.get(key); }
  acquire(key) {
    const lease = { key, value: this.values.get(key) };
    this.leases.set(lease, true);
    return lease;
  }
  remove(key) { this.values.delete(key); }
  release(lease) {
    this.leases.delete(lease);
  }
  subscribe(callback) {
    const token = {};
    this.listeners.set(token, callback);
    return token;
  }
  unsubscribe(token) { this.listeners.delete(token); }
  close() {
    this.values.clear();
  }
  retained() {
    return { leases: this.leases.size, listeners: this.listeners.size };
  }
}
