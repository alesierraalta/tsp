import { Store } from './store.js';
import { Audit } from './audit.js';
import { decodePatch, applyPatch, encodeSnapshot } from './codec.js';
import { exportCSV } from './export.js';
import { queryRows } from './query.js';
import { Client } from './client.js';

export class Service {
  constructor() {
    this.store = new Store();
    this.audit = new Audit();
  }

  request(request) {
    const { method, id, body } = request;
    switch (method) {
      case 'POST': {
        const row = this.store.create(JSON.parse(body));
        this.audit.record('create', row.id, this.store.revision);
        return row;
      }
      case 'GET':
        return this.store.read(id);
      case 'PATCH': {
        const patch = decodePatch(body);
        const row = this.store.replace(id, applyPatch(this.store.read(id), patch));
        this.audit.record('update', id, this.store.revision);
        return row;
      }
      case 'DELETE':
        this.store.remove(id);
        this.audit.record('remove', id, this.store.revision);
        return { removed: id };
      case 'EXPORT':
        return exportCSV(this.store.list(), request.columns, request.filter);
      case 'SNAPSHOT':
        return encodeSnapshot(this.store.list(), this.store.revision);
      default:
        throw new TypeError('unsupported method');
    }
  }

  query(options) {
    return queryRows(this.store.list(), options);
  }

  client() {
    return new Client(request => this.request(request));
  }
}
