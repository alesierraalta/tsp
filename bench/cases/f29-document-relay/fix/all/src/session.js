import { report } from './report.js';
import { options, identifier, payload } from './options.js';
import { Journal } from './journal.js';
import { Receipts } from './receipts.js';
import { Transport } from './transport.js';

export class Session {
  constructor(endpoint, input = {}, receipts = new Receipts()) {
    this.settings = options(input);
    this.journal = new Journal();
    this.receipts = receipts;
    this.transport = new Transport(endpoint, this.settings);
    this.busy = false;
  }

  enqueue(id, text) {
    identifier(id);
    if (this.receipts.get(id)) throw new Error('already delivered');
    return this.journal.append(id, payload(text, this.settings.maxBytes));
  }

  async flush() {
    if (this.busy) throw new Error('flush already in progress');
    this.busy = true;
    const delivered = [];
    try {
      for (const entry of this.journal.pending()) {
        const key = `${this.settings.namespace}/${entry.id}`;
        const receipt = await this.transport.upload(key, entry.bytes);
        const saved = await this.receipts.save(entry.id, receipt);
        this.journal.acknowledge(entry.id);
        delivered.push(saved);
      }
      return delivered;
    } finally {
      this.busy = false;
    }
  }

  status() {
    return {
      pending: this.journal.size(),
      delivered: this.receipts.stats().count,
      busy: this.busy,
    };
  }

  receipt(id) {
    return this.receipts.get(identifier(id));
  }

  report(offset = 0, limit = 20) {
    return report(this.journal.pending(), this.receipts.list(), offset, limit);
  }

  preview(text) {
    return this.transport.preview(payload(text, this.settings.maxBytes));
  }
}
