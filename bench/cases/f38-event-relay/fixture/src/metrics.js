export class Metrics {
  #opened = 0;
  #closed = 0;
  #events = 0;
  #pulses = 0;

  open() {
    this.#opened++;
  }

  close() {
    this.#closed++;
  }

  delivered() {
    this.#events++;
  }

  keepAlive() {
    this.#pulses++;
  }

  snapshot() {
    return {
      opened: this.#opened,
      closed: this.#closed,
      active: this.#opened - this.#closed,
      events: this.#events,
      keepAlives: this.#pulses,
    };
  }
}
