export function parseUrl(raw) {
  const text = String(raw ?? '').trim();
  if (text === '') throw new TypeError('an absolute URL is required');
  let url;
  try {
    url = new URL(text);
  } catch {
    throw new TypeError(`not an absolute URL: ${text}`);
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new TypeError(`unsupported URL scheme: ${url.protocol}`);
  }
  return url;
}

export function tryParseUrl(raw) {
  try {
    return parseUrl(raw);
  } catch {
    return undefined;
  }
}

export function hostOf(raw) {
  return parseUrl(raw).hostname;
}

export function hostPort(raw) {
  const url = parseUrl(raw);
  return url.port === '' ? url.hostname : `${url.hostname}:${url.port}`;
}

export function pathOf(raw) {
  const url = parseUrl(raw);
  return `${url.pathname}${url.search}`;
}

export function originOf(raw) {
  const url = parseUrl(raw);
  return `${url.protocol}//${url.host}`;
}

export function isAbsolute(raw) {
  return tryParseUrl(raw) !== undefined;
}

export function isHttps(raw) {
  return tryParseUrl(raw)?.protocol === 'https:';
}

export function sameOrigin(left, right) {
  const a = tryParseUrl(left);
  const b = tryParseUrl(right);
  if (a === undefined || b === undefined) return false;
  return a.protocol === b.protocol && a.host === b.host;
}

export function withoutCredentials(raw) {
  const url = parseUrl(raw);
  url.username = '';
  url.password = '';
  return url.href;
}

export function queryPairs(raw) {
  const url = parseUrl(raw);
  const out = [];
  for (const [name, value] of url.searchParams) out.push([name, value]);
  return out;
}

export function withQuery(raw, values = {}) {
  const url = parseUrl(raw);
  for (const [name, value] of Object.entries(values)) {
    if (value === undefined || value === null) url.searchParams.delete(name);
    else url.searchParams.set(name, String(value));
  }
  return url.href;
}

export function joinPath(base, ...parts) {
  const url = parseUrl(base);
  const segments = url.pathname.split('/').filter(Boolean);
  for (const part of parts) segments.push(...String(part ?? '').split('/').filter(Boolean));
  url.pathname = `/${segments.join('/')}`;
  return url.href;
}

export function normalizeHost(value) {
  return String(value ?? '')
    .trim()
    .toLowerCase()
    .replace(/\.$/, '');
}

export function describeTarget(raw) {
  const url = tryParseUrl(raw);
  if (url === undefined) return { valid: false };
  return {
    valid: true,
    host: url.hostname,
    port: url.port,
    protocol: url.protocol,
    path: url.pathname,
    hasCredentials: url.username !== '' || url.password !== '',
  };
}
