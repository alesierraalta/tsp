import { positive } from './quantity.js';

export class Basket {
  #lines = [];
  #sequence = 0;

  add(code, units = 1, note = '') {
    positive(units, 'units');
    if (typeof code !== 'string' || !code) {
      throw new TypeError('code required');
    }
    const id = ++this.#sequence;
    this.#lines.push({ id, code, units, note });
    return id;
  }

  remove(id) {
    const index = this.#lines.findIndex(line => line.id === id);
    if (index < 0) {
      return false;
    }
    this.#lines.splice(index, 1);
    return true;
  }

  lines() {
    return this.#lines.map(line => ({ ...line }));
  }

  get size() {
    return this.#lines.length;
  }

  clear() {
    this.#lines = [];
  }
}
