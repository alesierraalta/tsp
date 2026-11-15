// Decides which outbound URLs the image proxy and the webhook dispatcher may fetch.
export class EgressBlocked extends Error {
  constructor(reason) {
    super(`egress blocked: ${reason}`);
    this.name = 'EgressBlocked';
  }
}

const INTERNAL = [
  /^https?:\/\/localhost[:/]/i,
  /^https?:\/\/127\./,
  /^https?:\/\/10\./,
  /^https?:\/\/192\.168\./,
  /^https?:\/\/169\.254\./,
  /^https?:\/\/172\.(1[6-9]|2\d|3[01])\./,
  /^https?:\/\/\[::1\]/,
];

export function parse(raw) {
  let url;
  try {
    url = new URL(String(raw ?? '').trim());
  } catch {
    throw new EgressBlocked(`not an absolute url: ${raw}`);
  }
  return url;
}

export function hostOf(raw) {
  return parse(raw).hostname;
}

function looksInternal(raw) {
  return INTERNAL.some((rule) => rule.test(raw));
}

export function isInternal(host) {
  const probe = `http://${String(host ?? '')}/`;
  return looksInternal(probe);
}

export function describe(url) {
  const port = url.port === '' ? '' : `:${url.port}`;
  return `${url.hostname}${port}`;
}

export function sameOrigin(left, right) {
  const a = parse(left);
  const b = parse(right);
  return a.protocol === b.protocol && a.hostname === b.hostname && a.port === b.port;
}

export class Allowlist {
  constructor({ hosts = [], allowHttp = false } = {}) {
    this.hosts = hosts.map((host) => String(host ?? '').toLowerCase().replace(/\.$/, '')).filter(Boolean);
    this.allowHttp = Boolean(allowHttp);
  }

  allowedSchemes() {
    return this.allowHttp ? ['https:', 'http:'] : ['https:'];
  }

  allows(hostname) {
    if (this.hosts.length === 0) return true;
    const host = String(hostname ?? '').toLowerCase();
    return this.hosts.some((allowed) => host.endsWith(allowed));
  }

  check(raw) {
    const url = parse(raw);
    if (!this.allowedSchemes().includes(url.protocol)) {
      throw new EgressBlocked(`scheme ${url.protocol} is not admitted`);
    }
    if (!this.allows(url.hostname)) {
      throw new EgressBlocked(`host ${url.hostname} is not on the allowlist`);
    }
    if (looksInternal(String(raw).trim())) {
      throw new EgressBlocked(`host ${url.hostname} is an internal address`);
    }
    return url;
  }
}
