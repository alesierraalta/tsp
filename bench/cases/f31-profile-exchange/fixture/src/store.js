import { createRecord, identifier } from './schema.js';

export class Store {
  #rows = new Map();
  #revision = 0;

  create(input) {
    const record = createRecord(input);
    if (this.#rows.has(record.id)) throw new Error('duplicate id');
    this.#rows.set(record.id, record);
    this.#revision++;
    return this.read(record.id);
  }

  read(id) {
    const row = this.#rows.get(identifier(id));
    if (!row) throw new Error('record not found');
    return { ...row };
  }

  replace(id, row) {
    const key = identifier(id);
    this.read(key);
    const checked = createRecord({ ...row, id: key });
    this.#rows.set(key, checked);
    this.#revision++;
    return this.read(key);
  }

  remove(id) {
    const key = identifier(id);
    this.read(key);
    this.#rows.delete(key);
    this.#revision++;
  }

  list() {
    return [...this.#rows.values()].map(row => ({ ...row }));
  }

  get revision() {
    return this.#revision;
  }
}
