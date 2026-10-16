export class OrderConsumer {
  constructor(store) {
    this.store = store;
    this.seen = new Set();
  }

  handle(message) {
    const { id, orderId, version, status } = message;
    if (this.seen.has(id)) return "duplicate";
    const current = this.store.get(orderId);
    if (current && Number(current.version) > Number(version)) return "stale";
    this.store.apply(orderId, { version, status });
    this.seen.add(id);
    return "applied";
  }
}
