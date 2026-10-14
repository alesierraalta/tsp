import { copy, timestamp, page } from './values.js';

export class Journal {
  #events = [];
  #sequence = 0;

  constructor(clock = () => Date.now()) {
    this.clock = clock;
  }

  append(action, actor, resource, details = {}) {
    if (!action || !actor || !resource) throw new TypeError('event fields');
    const event = {
      sequence: ++this.#sequence,
      at: timestamp(this.clock()),
      action,
      actor,
      resource,
      details: copy(details),
    };
    this.#events.push(event);
    return copy(event);
  }

  list(options = {}) {
    const { offset, limit } = page(options);
    return this.#events.slice(offset, offset + limit).map(copy);
  }

  forResource(resource) {
    return this.#events.filter(event => event.resource === resource).map(copy);
  }

  counts() {
    const result = {};
    for (const event of this.#events) {
      result[event.action] = (result[event.action] ?? 0) + 1;
    }
    return result;
  }

  since(sequence) {
    if (!Number.isInteger(sequence) || sequence < 0) throw new TypeError('sequence');
    return this.#events.filter(event => event.sequence > sequence).map(copy);
  }

  get size() {
    return this.#events.length;
  }
}
