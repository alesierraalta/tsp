import { nonnegative, checkedAdd } from './quantity.js';

export class Ledger {
  #entries = [];
  #references = new Set();

  post(value) {
    nonnegative(value.credits, 'credits');
    if (!value.reference || this.#references.has(value.reference)) {
      throw new Error('unique reference required');
    }
    this.#entries.push({ reference: value.reference, credits: value.credits });
    this.#references.add(value.reference);
    return this.balance();
  }

  balance() {
    return this.#entries.reduce((sum, item) => checkedAdd(sum, item.credits), 0);
  }

  entries() {
    return this.#entries.map(item => ({ ...item }));
  }

  find(reference) {
    const item = this.#entries.find(entry => entry.reference === reference);
    return item ? { ...item } : null;
  }

  get size() {
    return this.#entries.length;
  }

  has(reference) {
    return this.#references.has(reference);
  }
}
