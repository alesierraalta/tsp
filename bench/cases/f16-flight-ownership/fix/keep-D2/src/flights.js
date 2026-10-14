export class FlightRegistry {
  constructor() {
    this.pending = new Map();
  }

  run(key, operation) {
    if (this.pending.has(key)) return this.pending.get(key);
    const promise = Promise.resolve().then(operation).finally(() => {
      this.pending.delete(key);
    });
    this.pending.set(key, promise);
    return promise;
  }

  invalidate(key) {
    this.pending.delete(key);
  }
}
