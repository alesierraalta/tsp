import { nonnegative, positive } from './quantity.js';

export class Catalog {
  #rates = new Map();

  define(code, microcredits, description = '') {
    if (typeof code !== 'string' || !code.trim()) {
      throw new TypeError('code required');
    }
    nonnegative(microcredits, 'rate');
    this.#rates.set(code, { code, microcredits, description });
    return this;
  }

  lookup(code) {
    const rate = this.#rates.get(code);
    if (!rate) {
      throw new Error(`unknown product: ${code}`);
    }
    return { ...rate };
  }

  quote(code, units = 1) {
    positive(units, 'units');
    const rate = this.lookup(code);
    const value = rate.microcredits * units;
    if (!Number.isSafeInteger(value)) {
      throw new RangeError('quote too large');
    }
    return value;
  }

  entries() {
    return [...this.#rates.values()].map(rate => ({ ...rate }));
  }

  has(code) {
    return this.#rates.has(code);
  }
}
