import { forbidden, badRequest } from '../util/errors.js';
import { parseUrl, hostOf } from '../util/url.js';
import { signDelivery, headerName } from './signature.js';
import { hostAllowed } from './targets.js';
import { newId } from '../util/ids.js';

export const DEFAULT_HEADERS = {
  'content-type': 'application/json',
  'user-agent': 'release-dashboard/1.0',
};

const RAW_INTERNAL = [
  /^https?:\/\/localhost[:/]/i,
  /^https?:\/\/127\./,
  /^https?:\/\/10\./,
  /^https?:\/\/192\.168\./,
  /^https?:\/\/169\.254\./,
  /^https?:\/\/172\.(1[6-9]|2\d|3[01])\./,
  /^https?:\/\/\[::1\]/i,
];

export class WebhookSender {
  constructor(options = {}) {
    this.secrets = options.secrets;
    this.transport = options.transport ?? globalThis.fetch;
    if (typeof this.transport !== 'function') throw new TypeError('a transport function is required');
    this.clock = options.clock ?? (() => new Date());
    this.timeoutMs = options.timeoutMs ?? 5000;
    this.allowed = options.allowed;
  }

  headersFor(delivery, body) {
    const headers = { ...DEFAULT_HEADERS, ...(delivery.headers ?? {}) };
    const secret = this.secrets?.secretFor(delivery.subscription) ?? delivery.secret;
    if (secret) headers[headerName()] = signDelivery(body, secret, new Date(this.clock()).getTime());
    return headers;
  }

  prepare(delivery) {
    const raw = String(delivery?.url ?? '');
    if (RAW_INTERNAL.some((pattern) => pattern.test(raw))) {
      throw forbidden(`egress blocked: ${hostOf(raw)} is an internal address`);
    }
    let url;
    try {
      url = parseUrl(raw);
    } catch (error) {
      throw badRequest('invalid_target', error.message);
    }
    if (url.protocol !== 'https:') throw forbidden(`egress blocked: scheme ${url.protocol} is not admitted`);
    if (!hostAllowed(url.hostname, this.allowed)) {
      throw forbidden(`egress blocked: ${url.hostname} is not an allowlisted host`);
    }
    const body = JSON.stringify(delivery.payload ?? {});
    return {
      url,
      href: url.href,
      host: url.port === '' ? url.hostname : `${url.hostname}:${url.port}`,
      hostname: url.hostname,
      headers: this.headersFor(delivery, body),
      body,
      key: delivery.id ?? newId('dlv'),
    };
  }

  async send(delivery) {
    const plan = this.prepare(delivery);
    let response;
    try {
      response = await this.transport(plan.href, {
        method: 'POST',
        headers: plan.headers,
        body: plan.body,
        signal: AbortSignal.timeout(this.timeoutMs),
      });
    } catch (error) {
      throw badRequest('delivery_failed', `post to ${plan.host} failed: ${error.message}`);
    }
    if (!response || response.status >= 400) {
      throw badRequest('delivery_failed', `post to ${plan.host} answered ${response?.status ?? 'nothing'}`);
    }
    return { status: response.status, host: plan.host, key: plan.key, headers: plan.headers };
  }

  describe() {
    return { transport: this.transport.name || 'anonymous', timeoutMs: this.timeoutMs, allowed: this.allowed ?? [] };
  }
}

export function recordingTransport() {
  const posted = [];
  return {
    posted,
    transport: async (href, options = {}) => {
      posted.push({ href, headers: options.headers ?? {} });
      return { status: 202, headers: new Headers() };
    },
  };
}

export function dryRunTransport() {
  return async () => ({ status: 204 });
}
