export class OrderConsumer {
  constructor(store) {
    this.store = store;
    this.seen = new Set();
  }

  handle(message) {
    const { id, orderId, version, status } = message;
    if (this.seen.has(id)) return "duplicate";
    this.seen.add(id);
    const current = this.store.get(orderId);
    if (current && current.version > version) return "stale";
    this.store.apply(orderId, { version, status });
    return "applied";
  }
}
