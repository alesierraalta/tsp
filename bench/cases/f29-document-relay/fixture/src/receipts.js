export class Receipts {
  constructor(beforeSave = async () => {}) {
    this.beforeSave = beforeSave;
    this.rows = new Map();
    this.revision = 0;
  }

  async save(id, receipt) {
    await this.beforeSave(id, receipt);
    const existing = this.rows.get(id);
    if (existing && existing.checksum !== receipt.checksum) {
      throw new Error('receipt conflicts with existing object');
    }
    const row = Object.freeze({ ...receipt, revision: ++this.revision });
    this.rows.set(id, row);
    return { ...row };
  }

  get(id) {
    const row = this.rows.get(id);
    return row ? { ...row } : null;
  }

  list() {
    return [...this.rows.entries()]
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([id, row]) => ({ id, ...row }));
  }

  stats() {
    return {
      count: this.rows.size,
      bytes: [...this.rows.values()].reduce((sum, row) => sum + row.bytes, 0),
      revision: this.revision,
    };
  }
}
