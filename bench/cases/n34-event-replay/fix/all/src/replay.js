const RETENTION_MS = 5 * 60 * 1000;

function isExpired(event) {
  return Date.now() - event.at > RETENTION_MS;
}

export class Replay {
  #state = null;
  #nextSequence = 1;
  #pending = new Map();

  get state() {
    return this.#state;
  }

  apply(event) {
    if (isExpired(event)) return;
    this.#applyArrival(event);
  }

  #applyArrival(event) {
    if (event.seq < this.#nextSequence) return;
    this.#pending.set(event.seq, event);
    while (this.#pending.has(this.#nextSequence)) {
      const next = this.#pending.get(this.#nextSequence);
      this.#pending.delete(this.#nextSequence);
      this.#apply(next);
      this.#nextSequence++;
    }
  }

  #apply(event) {
    this.#state = event.value;
  }
}
