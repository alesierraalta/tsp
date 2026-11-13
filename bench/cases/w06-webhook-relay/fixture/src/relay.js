import { DeliveryStore } from './queue/store.js';
import { OffsetStore } from './queue/offsets.js';
import { EgressRules } from './egress/rules.js';
import { parseAllowlist } from './egress/allowlist.js';
import { assertTarget } from './egress/url.js';
import { WebhookClient } from './egress/webhook.js';
import { KeyStore, createKey } from './auth/keys.js';
import { planRotation } from './auth/rotation.js';
import { Dispatcher } from './jobs/dispatcher.js';
import { HookRegistry } from './jobs/registry.js';
import { DEFAULT_TOPICS, TopicRegistry } from './jobs/topics.js';
import { runPostImportHook } from './jobs/hooks.js';
import { AuditLog } from './observability/audit.js';
import { reportFor, digestSummary } from './observability/report.js';
import { Metrics } from './observability/metrics.js';
import { envelope, assertPayloadSize } from './webhook/payload.js';
import { loadConfig } from './config.js';
import { silentLogger } from './util/log.js';
import { notFound } from './util/errors.js';

export class Relay {
  constructor(options) {
    this.store = options.store;
    this.rules = options.rules;
    this.keys = options.keys;
    this.client = options.client;
    this.dispatcher = options.dispatcher;
    this.hooks = options.hooks;
    this.topics = options.topics;
    this.audit = options.audit;
    this.offsets = options.offsets;
    this.config = options.config;
    this.clock = options.clock ?? (() => new Date());
    this.logger = options.logger ?? silentLogger();
    this.metrics = options.metrics ?? new Metrics();
  }

  enqueue(input) {
    const url = assertTarget(input.url, this.rules);
    const topic = this.topics.get(input.topic ?? 'default');
    const payload = input.payload ?? {};
    assertPayloadSize(payload);
    const row = this.store.insert({
      event: input.event ?? topic.event,
      topic: topic.name,
      target: input.target ?? topic.target,
      url: url.href,
      entity_id: input.entity_id,
      payload: envelope(input.event ?? topic.event, payload, { topic: topic.name, now: this.clock() }),
      headers: input.headers,
    });
    this.metrics.increment('deliveries.queued');
    this.audit.record('queued', { delivery_id: row.id, host: url.hostname });
    this.logger.info('delivery queued', { id: row.id, host: url.hostname });
    return row;
  }

  async deliverNow(id) {
    const row = this.store.get(id);
    this.store.markAttempt(id, undefined);
    this.metrics.increment('deliveries.attempted');
    this.audit.record('attempted', { delivery_id: id, host: row.target });
    try {
      const receipt = await this.client.send(row);
      this.store.markDelivered(id, receipt);
      this.metrics.increment('deliveries.delivered');
      this.audit.record('delivered', { delivery_id: id, host: receipt.host });
      return receipt;
    } catch (error) {
      this.store.markFailed(id, error);
      this.metrics.increment('deliveries.failed');
      this.audit.record('failed', { delivery_id: id, host: row.target, detail: error.code });
      throw error;
    }
  }

  retryNow(id) {
    const row = this.store.get(id);
    if (row.status === 'delivered') throw notFound(`delivery ${id} is already delivered`);
    const updated = this.store.update(id, {
      status: 'pending',
      next_attempt_at: new Date(this.clock()).toISOString(),
    });
    this.metrics.increment('deliveries.requeued');
    this.audit.record('requeued', { delivery_id: id, host: row.target });
    return updated;
  }

  runOnce() {
    return this.dispatcher.runOnce();
  }

  drain(maxBatches = 5) {
    return this.dispatcher.drain(maxBatches);
  }

  hookForImport(payloadName, options = {}) {
    this.audit.record('hook', { detail: payloadName, actor: 'import' });
    return runPostImportHook(this.hooks, payloadName, options);
  }

  rotation() {
    return planRotation(this.keys.snapshot(), { maxAgeDays: this.config.keyMaxAgeDays, graceHours: 6 }, this.clock());
  }

  stats() {
    return {
      counts: this.store.counts(),
      due: this.store.due(this.clock()).length,
      hooks: this.hooks.count(),
      topics: this.topics.count(),
      offsets: this.offsets.count(),
      audit: this.audit.counts(),
      rotation: this.rotation(),
      metrics: this.metrics.snapshot(),
    };
  }


  bulkEnqueue(inputs) {
    if (!Array.isArray(inputs)) throw new TypeError('bulkEnqueue expects a list');
    return inputs.map((input) => this.enqueue(input));
  }

  deadLetters(limit = 50) {
    return this.store.list({ status: 'failed', limit });
  }

  reprocessDeadLetters(limit = 25) {
    const rows = this.deadLetters(limit);
    return rows.map((row) => this.retryNow(row.id));
  }

  digest(options = {}) {
    return reportFor(this.store.list({ limit: options.limit ?? 500 }), {
      now: this.clock(),
      staleSeconds: options.staleSeconds,
    });
  }

  digestText(options = {}) {
    return digestSummary(this.digest(options).digest);
  }

  auditTail(count = 10) {
    return this.audit.tail(count);
  }

  topicsFor(event) {
    return this.topics.forEvent(event);
  }

  pending() {
    return this.store.list({ status: 'pending' });
  }

  describe() {
    return {
      egress: this.rules.describe(),
      config: this.config,
      hooks: this.hooks.describe(),
      topics: this.topics.describe(),
      audit: this.audit.tail(5),
    };
  }
}

export function createRelay(options = {}) {
  const config = loadConfig(options.config ?? {});
  const clock = options.clock ?? (() => new Date());
  const logger = options.logger ?? silentLogger();
  const metrics = options.metrics ?? new Metrics({ clock: () => new Date(clock()).getTime() });
  const store = options.store ?? new DeliveryStore({ clock });
  const rules =
    options.rules ??
    new EgressRules({
      allowlist: parseAllowlist(config.allowlist),
      allowInternal: config.allowInternalHosts,
      allowInsecure: config.allowInsecure,
    });
  const keys = options.keys ?? new KeyStore({ clock, seed: createKey({ now: clock() }) });
  const client =
    options.client ??
    new WebhookClient({
      rules,
      keys,
      transport: options.transport,
      clock,
      timeoutMs: config.timeoutMs,
    });
  const dispatcher = new Dispatcher({ store, client, rules, config, clock, logger, metrics });
  const hooks = options.hooks ?? new HookRegistry();
  const topics = options.topics ?? new TopicRegistry(DEFAULT_TOPICS);
  const audit = options.audit ?? new AuditLog({ clock });
  const offsets = new OffsetStore();
  return new Relay({
    store,
    rules,
    keys,
    client,
    dispatcher,
    hooks,
    topics,
    audit,
    offsets,
    config,
    clock,
    logger,
    metrics,
  });
}

export function sampleDelivery(overrides = {}) {
  return {
    event: 'order.created',
    topic: 'orders',
    url: 'https://hooks.example.com/relay',
    entity_id: 'ord_1',
    payload: { id: 'ord_1' },
    ...overrides,
  };
}
