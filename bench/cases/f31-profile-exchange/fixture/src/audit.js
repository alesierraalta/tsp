export class Audit {
  #events = [];
  #sequence = 0;

  record(action, id, revision) {
    if (typeof action !== 'string' || !action) throw new TypeError('invalid action');
    if (!Number.isSafeInteger(revision) || revision < 0) throw new TypeError('invalid revision');
    const event = Object.freeze({ sequence: ++this.#sequence, action, id, revision });
    this.#events.push(event);
    return { ...event };
  }

  since(sequence = 0) {
    if (!Number.isSafeInteger(sequence) || sequence < 0) throw new TypeError('invalid sequence');
    return this.#events.filter(event => event.sequence > sequence).map(event => ({ ...event }));
  }

  forRecord(id) {
    return this.#events.filter(event => event.id === id).map(event => ({ ...event }));
  }

  summary() {
    const actions = {};
    for (const event of this.#events) {
      actions[event.action] = (actions[event.action] ?? 0) + 1;
    }
    return { count: this.#events.length, actions };
  }

  get latest() {
    return this.#sequence;
  }
}
