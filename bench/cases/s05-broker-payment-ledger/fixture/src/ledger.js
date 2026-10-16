const tick = () => new Promise((resolve) => setImmediate(resolve));

// The ledger is a remote store: every read and write takes a round trip.
export class Ledger {
  constructor() {
    this.balances = new Map();
  }

  async read(account) {
    await tick();
    return this.balances.get(account) ?? 0;
  }

  async write(account, balance) {
    await tick();
    this.balances.set(account, balance);
  }
}
