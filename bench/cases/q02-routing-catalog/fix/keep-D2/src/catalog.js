export class Catalog {
  constructor({ backends = ['east', 'west'], store = new Map(), lookup } = {}) {
    this.backends = [...backends];
    this.store = store;
    this.lookup = lookup ?? (async name => ({ ok: true, records: [{ name, address: `${name}.internal` }] }));
    this.selected = this.backends[0];
    this.rows = this.store.get('rows') ?? [];
  }

  choose(name) {
    if (!this.backends.includes(name)) return false;
    this.selected = name;
    this.store.set('selected', name);
    return true;
  }

  async rebuild() {
    const rows = [];
    for (const name of this.backends) {
      const reply = await this.lookup(name);
      if (!reply.ok) return false;
      rows.push(...reply.records.map(record => ({ ...record, backend: name })));
    }
    this.rows = rows;
    this.store.set('rows', rows);
    return true;
  }

  route() {
    return this.rows.filter(row => row.backend === this.selected).map(row => ({ ...row }));
  }
}
