import { encodePatch } from './codec.js';
import { compactRecord } from './normalize.js';
import { mutable, identifier, checkFields } from './schema.js';

export class Client {
  constructor(transport) {
    if (typeof transport !== 'function') throw new TypeError('transport required');
    this.transport = transport;
  }

  create(record) {
    return this.transport({ method: 'POST', body: JSON.stringify(compactRecord(record)) });
  }

  read(id) {
    return this.transport({ method: 'GET', id: identifier(id) });
  }

  update(id, changes) {
    checkFields(changes, mutable);
    const payload = compactRecord(changes);
    return this.transport({
      method: 'PATCH',
      id: identifier(id),
      body: encodePatch(payload),
    });
  }

  remove(id) {
    return this.transport({ method: 'DELETE', id: identifier(id) });
  }

  export(columns, filter = {}) {
    return this.transport({ method: 'EXPORT', columns, filter });
  }

  snapshot() {
    return this.transport({ method: 'SNAPSHOT' });
  }
}
