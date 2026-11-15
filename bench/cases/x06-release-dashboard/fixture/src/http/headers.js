export function parseHeaders(raw = {}) {
  const out = {};
  for (const [name, value] of Object.entries(raw)) {
    out[name.toLowerCase()] = Array.isArray(value) ? value.join(', ') : String(value ?? '');
  }
  return out;
}

export function headerOf(headers, name, fallback = '') {
  const value = headers?.[String(name).toLowerCase()];
  return value === undefined ? fallback : value;
}

export function bearerOf(headers) {
  const match = /^Bearer\s+(.+)$/i.exec(headerOf(headers, 'authorization').trim());
  return match ? match[1].trim() : '';
}

export function apiKeyOf(headers) {
  const direct = headerOf(headers, 'x-api-key').trim();
  if (direct !== '') return direct;
  return bearerOf(headers);
}

export function contentTypeOf(headers) {
  return headerOf(headers, 'content-type').split(';')[0].trim().toLowerCase();
}

export function contentLengthOf(headers) {
  const value = Number(headerOf(headers, 'content-length', '0'));
  return Number.isFinite(value) && value > 0 ? Math.trunc(value) : 0;
}

export function isJson(headers) {
  return contentTypeOf(headers) === 'application/json';
}

export function accepts(headers, type) {
  const value = headerOf(headers, 'accept');
  if (value === '' || value === '*/*') return true;
  return value.split(',').some((part) => {
    const candidate = part.split(';')[0].trim().toLowerCase();
    return candidate === type || candidate === '*/*' || (candidate.endsWith('/*') && type.startsWith(candidate.slice(0, -1)));
  });
}

export function requestIdOf(headers) {
  const provided = headerOf(headers, 'x-request-id').trim();
  return /^[A-Za-z0-9-]{8,64}$/.test(provided) ? provided : undefined;
}

export function headerList(headers) {
  return Object.keys(headers ?? {}).sort();
}

export function signatureHeaderOf(headers) {
  return headerOf(headers, 'x-dashboard-signature').trim();
}
