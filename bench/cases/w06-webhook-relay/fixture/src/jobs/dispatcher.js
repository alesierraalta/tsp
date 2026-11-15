import { assertTarget } from '../egress/url.js';
import { classify, scheduleRetry } from './retry.js';
import { asRelayError } from '../util/errors.js';
import { isOpen, parseWindow } from '../queue/window.js';

export class Dispatcher {
  constructor(options = {}) {
    this.store = options.store;
    this.client = options.client;
    this.rules = options.rules;
    this.config = options.config;
    this.clock = options.clock ?? (() => new Date());
    this.logger = options.logger;
    this.metrics = options.metrics;
    this.window = parseWindow(this.config?.deliveryWindow ?? '00:00-23:59');
  }

  async attempt(row) {
    let target;
    try {
      target = assertTarget(row.url, this.rules);
    } catch (error) {
      this.store.markFailed(row.id, error);
      this.metrics?.increment('deliveries.blocked');
      throw error;
    }
    this.store.markAttempt(row.id, undefined);
    this.metrics?.increment('deliveries.attempted');
    try {
      const receipt = await this.client.send({ ...row, url: target.href });
      this.store.markDelivered(row.id, receipt);
      this.metrics?.increment('deliveries.delivered');
      return { id: row.id, status: 'delivered', receipt };
    } catch (error) {
      const failure = asRelayError(error);
      const verdict = classify(failure);
      this.metrics?.increment('deliveries.failed');
      if (!verdict.retryable) {
        this.store.markFailed(row.id, failure);
        return { id: row.id, status: 'failed', error: failure.message };
      }
      const scheduled = scheduleRetry(this.store, row.id, failure, this.config, { now: this.clock() });
      this.logger?.warn('delivery retry', { id: row.id, at: scheduled.next_attempt_at ?? null });
      return { id: row.id, status: scheduled.status, next_attempt_at: scheduled.next_attempt_at };
    }
  }

  async runOnce() {
    if (!isOpen(this.window, this.clock())) {
      this.metrics?.increment('deliveries.deferred');
      return [];
    }
    const batch = this.store.due(this.clock()).slice(0, this.config.batchSize);
    const results = [];
    for (const row of batch) {
      try {
        results.push(await this.attempt(row));
      } catch (error) {
        results.push({ id: row.id, status: 'blocked', error: error.message });
      }
    }
    this.metrics?.gauge('deliveries.due', this.store.due(this.clock()).length);
    return results;
  }

  async drain(maxBatches = 5) {
    const all = [];
    for (let round = 0; round < maxBatches; round += 1) {
      const results = await this.runOnce();
      if (results.length === 0) break;
      all.push(...results);
    }
    return all;
  }

  pending() {
    return this.store.due(this.clock());
  }
}
