import { EgressError, DeliveryError } from '../util/errors.js';
import { parseUrl, hostOf } from '../util/url.js';
import { idempotencyKey } from '../util/ids.js';
import { signMessage } from '../auth/signature.js';

export const DEFAULT_HEADERS = {
  'content-type': 'application/json',
  'user-agent': 'relay/1.0',
};

export class WebhookClient {
  constructor(options = {}) {
    this.rules = options.rules;
    if (!this.rules) throw new TypeError('egress rules are required');
    this.keys = options.keys;
    this.transport = options.transport ?? globalThis.fetch;
    if (typeof this.transport !== 'function') throw new TypeError('a transport function is required');
    this.clock = options.clock ?? (() => new Date());
    this.timeoutMs = options.timeoutMs ?? 5000;
  }

  headersFor(delivery) {
    const headers = { ...DEFAULT_HEADERS, ...(delivery.headers ?? {}) };
    if (this.keys) {
      const key = this.keys.active();
      const body = JSON.stringify(delivery.payload ?? {});
      const timestamp = Math.floor(new Date(this.clock()).getTime() / 1000);
      headers['x-relay-signature'] = signMessage(body, key.secret, timestamp);
      headers['x-relay-key'] = key.id;
    }
    return headers;
  }

  prepare(delivery) {
    const raw = String(delivery?.url ?? '');
    if (this.rules.blocksInternal(raw)) {
      throw new EgressError(`${hostOf(raw)} is an internal address`);
    }
    let url;
    try {
      url = parseUrl(raw);
    } catch (error) {
      throw new EgressError(error.message);
    }
    if (!this.rules.allowsScheme(url.protocol)) {
      throw new EgressError(`scheme ${url.protocol} is not admitted`);
    }
    if (!this.rules.allowsHost(url.hostname)) {
      throw new EgressError(`host ${url.hostname} is not on the allowlist`);
    }
    return {
      url,
      href: url.href,
      host: url.hostname,
      headers: this.headersFor(delivery),
      key: idempotencyKey(delivery),
    };
  }

  async send(delivery) {
    const plan = this.prepare(delivery);
    const body = JSON.stringify(delivery.payload ?? {});
    let response;
    try {
      response = await this.transport(plan.href, {
        method: 'POST',
        headers: plan.headers,
        body,
        signal: AbortSignal.timeout(this.timeoutMs),
      });
    } catch (error) {
      throw new DeliveryError(`post to ${plan.host} failed: ${error.message}`);
    }
    if (!response || response.status >= 400) {
      throw new DeliveryError(`post to ${plan.host} answered ${response?.status ?? 'nothing'}`, {
        status: response?.status,
      });
    }
    return {
      status: response.status,
      host: plan.host,
      key: plan.key,
      headers: plan.headers,
      body,
    };
  }

  describe() {
    return { transport: this.transport.name || 'anonymous', timeoutMs: this.timeoutMs };
  }
}
