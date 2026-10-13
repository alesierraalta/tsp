const RETENTION_MS = 5 * 60 * 1000;

function isExpired(event) {
  return Date.now() - event.at > RETENTION_MS;
}

export class Replay {
  #state = null;

  get state() {
    return this.#state;
  }

  apply(event) {
    if (isExpired(event)) return;
    this.#applyArrival(event);
  }

  #applyArrival(event) {
    this.#apply(event);
  }

  #apply(event) {
    this.#state = event.value;
  }
}
