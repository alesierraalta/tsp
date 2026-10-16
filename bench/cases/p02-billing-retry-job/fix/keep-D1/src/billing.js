export class BillingJob {
  constructor(payments, ledger) {
    this.payments = payments;
    this.ledger = ledger;
  }

  run(job) {
    if (this.ledger.completed(job.id)) return 'complete';
    const minorUnits = Math.round(job.amount * 100);
    const receipt = this.payments.charge(job.account, minorUnits);
    const stored = this.ledger.commit(job, receipt);
    return stored ? 'complete' : 'retry';
  }
}

export class MemoryPayments {
  constructor() {
    this.charges = [];
    this.receipts = new Map();
  }

  charge(account, minorUnits, key) {
    if (key !== undefined && this.receipts.has(key)) {
      return this.receipts.get(key);
    }
    const receipt = `receipt-${this.charges.length + 1}`;
    this.charges.push({ account, minorUnits, receipt });
    if (key !== undefined) this.receipts.set(key, receipt);
    return receipt;
  }
}

export class MemoryLedger {
  constructor(accept = () => true) {
    this.accept = accept;
    this.balances = new Map();
    this.jobs = new Map();
  }

  completed(id) {
    return this.jobs.has(id);
  }

  balance(account) {
    return this.balances.get(account) ?? 0;
  }

  commit(job, receipt) {
    if (this.completed(job.id)) return true;
    if (!this.accept(job, receipt)) return false;
    this.balances.set(job.account, this.balance(job.account) + job.amount);
    this.jobs.set(job.id, receipt);
    return true;
  }
}
