const SCHEMES = new Set(['https:']);

export function normaliseHost(value) {
  return String(value ?? '')
    .trim()
    .toLowerCase()
    .replace(/\.$/, '');
}

const INTERNAL_RULES = [
  /^https?:\/\/localhost[:/]/i,
  /^https?:\/\/127\./,
  /^https?:\/\/10\./,
  /^https?:\/\/192\.168\./,
  /^https?:\/\/169\.254\./,
  /^https?:\/\/172\.(1[6-9]|2\d|3[01])\./,
  /^https?:\/\/\[::1\]/i,
  /^https?:\/\/0\.0\.0\.0[:/]/,
];

export class EgressRules {
  constructor(options = {}) {
    this.allowlist = (options.allowlist ?? [])
      .map((host) => normaliseHost(host))
      .filter((host) => host.length > 0);
    this.allowInternal = Boolean(options.allowInternal);
    this.allowInsecure = Boolean(options.allowInsecure);
  }

  allowedSchemes() {
    return this.allowInsecure ? new Set([...SCHEMES, 'http:']) : new Set(SCHEMES);
  }

  allowsScheme(protocol) {
    return this.allowedSchemes().has(String(protocol).toLowerCase());
  }

  allowsHost(hostname) {
    if (this.allowlist.length === 0) return true;
    const host = normaliseHost(hostname);
    return this.allowlist.some((allowed) => host === allowed || host.endsWith(`.${allowed}`));
  }

  blocksInternal(target) {
    if (this.allowInternal) return false;
    const text = String(target ?? '');
    return INTERNAL_RULES.some((rule) => rule.test(text));
  }

  describe() {
    if (this.allowlist.length === 0) return 'any public host';
    return this.allowlist.join(', ');
  }
}

export function isAllowedHost(rules, hostname) {
  return rules.allowsHost(hostname);
}

export function internalRules() {
  return [...INTERNAL_RULES];
}

export function withAllowlist(rules, hosts) {
  return new EgressRules({
    allowlist: hosts,
    allowInternal: rules.allowInternal,
    allowInsecure: rules.allowInsecure,
  });
}

export function describeRules(rules) {
  const schemes = [...rules.allowedSchemes()].join('+');
  return `${schemes} hosts=${rules.describe()} internal=${rules.allowInternal ? 'allowed' : 'blocked'}`;
}

export function isPublicTarget(rules, hostname) {
  return !rules.blocksInternal(`https://${String(hostname ?? '')}/`);
}
