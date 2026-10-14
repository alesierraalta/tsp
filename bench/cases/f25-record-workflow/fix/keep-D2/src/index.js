export class Records {
  #rows = new Map();
  #jobs = new Map();
  #history = new Map();
  create(id, owner, title) {
    this.#rows.set(id, { owner, title });
  }
  read(id) {
    const row = this.#rows.get(id);
    return row ? { ...row } : null;
  }
  transfer(actor, id, owner) {
    const row = this.#owned(actor, id);
    row.owner = owner;
  }
  rename(actor, id, title) {
    const row = this.#owned(actor, id);
    this.#history.set(id, { actor, title: row.title });
    row.title = title;
  }
  prepareRename(actor, ids, title) {
    for (const id of ids) this.#owned(actor, id);
    const ticket = Symbol();
    this.#jobs.set(ticket, { actor, ids: [...ids], title });
    return ticket;
  }
  applyRename(ticket) {
    const job = this.#jobs.get(ticket);
    if (!job) throw new Error('unknown ticket');
    const rows = job.ids.map(id => this.#owned(job.actor, id));
    for (const row of rows) if (!row) throw new Error('missing record');
    for (const row of rows) row.title = job.title;
    this.#jobs.delete(ticket);
  }
  undoRename(actor, id) {
    const history = this.#history.get(id);
    if (!history || history.actor !== actor) throw new Error('denied');
    const row = this.#rows.get(id);
    if (!row) throw new Error('missing record');
    row.title = history.title;
    this.#history.delete(id);
  }
  #owned(actor, id) {
    const row = this.#rows.get(id);
    if (!row || row.owner !== actor) throw new Error('denied');
    return row;
  }
}
