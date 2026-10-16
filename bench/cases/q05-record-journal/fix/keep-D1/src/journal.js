export class Journal {
  constructor({ columns = ['key', 'value'], state = { next: 0, entries: new Map() } } = {}) {
    this.columns = [...columns];
    this.width = Math.max(2, columns.length);
    this.state = state;
    this.rows = new Map();
  }

  configure(columns) {
    this.columns = [...columns];
  }

  ingest(fields) {
    if (fields.length < this.width) return false;
    const row = {};
    for (let i = 0; i < this.columns.length; i++) row[this.columns[i]] = fields[i] ?? '';
    this.rows.set(fields[0], row);
    return true;
  }

  begin(value) {
    if (this.state.entries.size >= 65536) return null;
    let id = this.state.next;
    while (this.state.entries.has(id)) id = (id + 1) & 0xffff;
    this.state.next = (id + 1) & 0xffff;
    this.state.entries.set(id, value);
    return id;
  }

  commit(id) {
    const value = this.state.entries.get(id);
    this.state.entries.delete(id);
    return value;
  }
}
