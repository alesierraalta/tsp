import { forbidden, badRequest } from '../util/errors.js';

export const ALLOWED_HOSTS = ['hooks.example.com', 'ingest.example.com', 'studio-metrics.example.com'];

const INTERNAL_PATTERNS = [/^127\./, /^10\./, /^192\.168\./, /^169\.254\./, /^::1$/, /^localhost$/];

export function isInternalHost(hostname) {
  const host = String(hostname ?? '').toLowerCase();
  return INTERNAL_PATTERNS.some((pattern) => pattern.test(host));
}

export function hostAllowed(hostname, allowed = ALLOWED_HOSTS) {
  const host = String(hostname ?? '').toLowerCase();
  return allowed.some((entry) => host === entry || host.endsWith(`.${entry}`));
}

export function parseTarget(raw) {
  const text = String(raw ?? '').trim();
  if (text === '') throw badRequest('invalid_target', 'a target url is required');
  let parsed;
  try {
    parsed = new URL(text);
  } catch {
    throw badRequest('invalid_target', `${text} is not a url`);
  }
  if (parsed.protocol !== 'https:') throw badRequest('invalid_target', 'a target must use https');
  return parsed;
}

export function assertTarget(raw, options = {}) {
  const parsed = parseTarget(raw);
  if (isInternalHost(parsed.hostname) && options.allowInternal !== true) {
    throw forbidden(`egress blocked: ${parsed.hostname} is an internal address`);
  }
  const allowed = options.allowed ?? ALLOWED_HOSTS;
  if (!hostAllowed(parsed.hostname, allowed)) {
    throw forbidden(`egress blocked: ${parsed.hostname} is not an allowlisted host`);
  }
  return { host: parsed.hostname, path: parsed.pathname, url: parsed.toString() };
}

export function targetOf(raw, options = {}) {
  const checked = assertTarget(raw, options);
  return { ...checked, checked_at: new Date().toISOString() };
}

export function policyReport(options = {}) {
  return {
    allowed: [...(options.allowed ?? ALLOWED_HOSTS)],
    internal: INTERNAL_PATTERNS.map((pattern) => pattern.source),
  };
}

export function hostOf(raw) {
  try {
    return new URL(String(raw)).hostname;
  } catch {
    return undefined;
  }
}
