import { HttpError } from './errors.js';

export function parseHeaders(raw = {}) {
  const headers = Object.create(null);
  for (const [key, value] of Object.entries(raw)) {
    if (value === undefined) continue;
    headers[key.toLowerCase()] = Array.isArray(value) ? value.join(', ') : String(value);
  }
  return headers;
}

export function contentType(headers) {
  const raw = headers['content-type'] ?? '';
  return raw.split(';')[0].trim().toLowerCase();
}

export function isJson(headers) {
  const type = contentType(headers);
  return type === 'application/json' || type.endsWith('+json');
}

export function isForm(headers) {
  return contentType(headers) === 'application/x-www-form-urlencoded';
}

export function acceptsJson(headers) {
  const accept = headers.accept ?? '*/*';
  return accept.includes('json') || accept.includes('*/*');
}

export function bearerToken(headers) {
  const raw = headers.authorization ?? '';
  const match = /^Bearer\s+(\S+)$/i.exec(raw.trim());
  if (!match) return undefined;
  const token = match[1];
  if (token.length > 4096) {
    throw new HttpError(401, 'unauthorized', 'token is too long');
  }
  return token;
}

export function ifNoneMatch(headers) {
  const raw = headers['if-none-match'] ?? '';
  return raw
    .split(',')
    .map((entry) => entry.trim().replace(/^W\//, '').replace(/^"|"$/g, ''))
    .filter(Boolean);
}

export function requestId(headers, fallback) {
  const raw = headers['x-request-id'];
  if (typeof raw === 'string' && /^[A-Za-z0-9._-]{1,64}$/.test(raw)) return raw;
  return fallback;
}

export function forwardedFor(headers) {
  const raw = headers['x-forwarded-for'];
  if (typeof raw !== 'string' || raw.length === 0) return undefined;
  return raw.split(',')[0].trim();
}

export function mergeVary(existing, value) {
  const parts = new Set(
    String(existing ?? '')
      .split(',')
      .map((entry) => entry.trim())
      .filter(Boolean),
  );
  parts.add(value);
  return [...parts].join(', ');
}

export function securityHeaders() {
  return {
    'x-content-type-options': 'nosniff',
    'x-frame-options': 'DENY',
    'referrer-policy': 'no-referrer',
    'cache-control': 'no-store',
  };
}
