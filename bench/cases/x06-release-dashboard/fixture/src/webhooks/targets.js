import { forbidden, badRequest } from '../util/errors.js';
import { normalizeHost, tryParseUrl } from '../util/url.js';

// An operator may pin the hosts this dashboard may post to. An empty allowlist admits any public host,
// which is the default: the guard that always runs is the internal-address one, evaluated on the host the
// URL actually reaches.
export const ALLOWED_HOSTS = [];

export const INTERNAL_PATTERNS = [
  /^127\./,
  /^10\./,
  /^192\.168\./,
  /^169\.254\./,
  /^172\.(1[6-9]|2[0-9]|3[01])\./,
  /^0\.0\.0\.0$/,
  /^::1$/,
  /^::$/,
  /^localhost$/,
  /^\[::1\]$/,
];

export function isInternalHost(hostname) {
  const host = normalizeHost(hostname);
  return INTERNAL_PATTERNS.some((pattern) => pattern.test(host));
}

export function hostAllowed(hostname, allowed = ALLOWED_HOSTS) {
  const host = normalizeHost(hostname);
  const list = (allowed ?? []).map(normalizeHost).filter((entry) => entry !== '');
  if (list.length === 0) return true;
  return list.some((entry) => host === entry || host.endsWith(`.${entry}`));
}

export function parseTarget(raw) {
  const text = String(raw ?? '').trim();
  if (text === '') throw badRequest('invalid_target', 'a target url is required');
  const parsed = tryParseUrl(text);
  if (parsed === undefined) throw badRequest('invalid_target', `${text} is not a url`);
  if (parsed.protocol !== 'https:') throw badRequest('invalid_target', 'a target must use https');
  return parsed;
}

export function assertTarget(raw, options = {}) {
  const parsed = parseTarget(raw);
  if (isInternalHost(parsed.hostname) && options.allowInternal !== true) {
    throw forbidden(`egress blocked: ${parsed.hostname} is an internal address`);
  }
  if (!hostAllowed(parsed.hostname, options.allowed ?? ALLOWED_HOSTS)) {
    throw forbidden(`egress blocked: ${parsed.hostname} is not an allowlisted host`);
  }
  return { host: parsed.hostname, port: parsed.port, path: parsed.pathname, url: parsed.toString() };
}

export function targetOf(raw, options = {}) {
  return { ...assertTarget(raw, options), checked_at: new Date().toISOString() };
}

export function targetSummary(target) {
  return { host: target.host, path: target.path };
}

export function policyReport(options = {}) {
  return {
    allowed: [...(options.allowed ?? ALLOWED_HOSTS)],
    internal: INTERNAL_PATTERNS.map((pattern) => pattern.source),
    scheme: 'https',
  };
}

export function hostOf(raw) {
  return tryParseUrl(raw)?.hostname;
}

export function describeDecision(raw, options = {}) {
  try {
    const target = assertTarget(raw, options);
    return { allowed: true, host: target.host, url: target.url };
  } catch (error) {
    return { allowed: false, host: hostOf(raw), reason: error.message };
  }
}
