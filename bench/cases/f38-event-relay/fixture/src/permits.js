export class PermitPool {
  #active = new Map();

  constructor(limit = 1) {
    if (!Number.isInteger(limit) || limit < 1) {
      throw new RangeError('positive permit limit required');
    }
    this.limit = limit;
  }

  acquire(subscriber) {
    const used = this.#active.get(subscriber) ?? 0;
    if (used >= this.limit) {
      const error = new Error('subscriber capacity reached');
      error.code = 'CAPACITY';
      throw error;
    }
    this.#active.set(subscriber, used + 1);
    let held = true;
    return () => {
      if (!held) return;
      held = false;
      const remaining = this.used(subscriber) - 1;
      if (remaining) this.#active.set(subscriber, remaining);
      else this.#active.delete(subscriber);
    };
  }

  used(subscriber) {
    return this.#active.get(subscriber) ?? 0;
  }

  snapshot() {
    return Object.fromEntries(this.#active);
  }
}
