export class Transaction {
  constructor(store) {
    this.store = store;
    this.snapshot = store.snapshot();
    this.finished = false;
  }

  commit() {
    this.finished = true;
    this.snapshot = undefined;
    return true;
  }

  rollback() {
    if (this.finished) return false;
    this.store.restore(this.snapshot);
    this.finished = true;
    return true;
  }
}

export async function runInTransaction(store, run) {
  const transaction = new Transaction(store);
  try {
    const result = await run(transaction);
    transaction.commit();
    return result;
  } catch (error) {
    transaction.rollback();
    throw error;
  }
}

export function runSync(store, run) {
  const transaction = new Transaction(store);
  try {
    const result = run(transaction);
    transaction.commit();
    return result;
  } catch (error) {
    transaction.rollback();
    throw error;
  }
}

export class WriteQueue {
  constructor() {
    this.pending = Promise.resolve();
    this.depth = 0;
  }

  push(run) {
    this.depth += 1;
    const next = this.pending.then(run, run).finally(() => {
      this.depth -= 1;
    });
    this.pending = next.catch(() => undefined);
    return next;
  }
}
