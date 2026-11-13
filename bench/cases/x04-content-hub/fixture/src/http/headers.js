export function parseHeaders(raw = {}) {
  const out = {};
  for (const [key, value] of Object.entries(raw)) {
    out[key.toLowerCase()] = Array.isArray(value) ? value.join(', ') : String(value ?? '');
  }
  return out;
}

export function headerOf(headers, name, fallback = '') {
  const value = headers?.[String(name).toLowerCase()];
  return value === undefined ? fallback : value;
}

export function bearerOf(headers) {
  const value = headerOf(headers, 'authorization');
  const match = /^Bearer\s+(.+)$/i.exec(value.trim());
  return match ? match[1].trim() : '';
}

export function contentTypeOf(headers) {
  const value = headerOf(headers, 'content-type');
  return value.split(';')[0].trim().toLowerCase();
}

export function contentTypeParams(headers) {
  const value = headerOf(headers, 'content-type');
  const out = {};
  for (const part of value.split(';').slice(1)) {
    const index = part.indexOf('=');
    if (index <= 0) continue;
    const key = part.slice(0, index).trim().toLowerCase();
    const raw = part.slice(index + 1).trim();
    out[key] = raw.replace(/^"|"$/g, '');
  }
  return out;
}

export function accepts(headers, type) {
  const value = headerOf(headers, 'accept');
  if (value === '' || value === '*/*') return true;
  return value
    .split(',')
    .map((part) => part.split(';')[0].trim().toLowerCase())
    .some((part) => part === type || part === '*/*' || (part.endsWith('/*') && type.startsWith(part.slice(0, -1))));
}

export function isJson(headers) {
  return contentTypeOf(headers) === 'application/json';
}

export function contentLengthOf(headers) {
  const value = Number(headerOf(headers, 'content-length', '0'));
  return Number.isFinite(value) && value > 0 ? Math.trunc(value) : 0;
}

export function negotiate(headers, supported) {
  const value = headerOf(headers, 'accept');
  const wanted = value
    .split(',')
    .map((part) => part.split(';')[0].trim().toLowerCase())
    .filter(Boolean);
  for (const type of wanted) {
    if (supported.includes(type)) return type;
    if (type.endsWith('/*')) {
      const family = type.slice(0, -1);
      const match = supported.find((item) => item.startsWith(family));
      if (match) return match;
    }
  }
  return supported[0];
}

export function agentOf(headers) {
  return headerOf(headers, 'user-agent', 'unknown');
}

export function requestIdOf(headers) {
  const provided = headerOf(headers, 'x-request-id').trim();
  if (/^[A-Za-z0-9-]{8,64}$/.test(provided)) return provided;
  return `req_${Math.random().toString(36).slice(2, 10)}`;
}

export function isSecure(headers) {
  return headerOf(headers, 'x-forwarded-proto') === 'https';
}

export function etagMatches(headers, etag) {
  const value = headerOf(headers, 'if-none-match');
  if (value === '') return false;
  return value.split(',').map((part) => part.trim()).includes(etag);
}

export function forwardHeaders(headers, names) {
  const out = {};
  for (const name of names) {
    const value = headerOf(headers, name);
    if (value !== '') out[name] = value;
  }
  return out;
}

export function headerSummary(headers) {
  return {
    keys: Object.keys(headers).length,
    json: contentTypeOf(headers) === 'application/json',
    authenticated: bearerOf(headers) !== '',
    secure: isSecure(headers),
  };
}
