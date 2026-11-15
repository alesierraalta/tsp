export function parseUrl(raw) {
  const text = String(raw ?? '').trim();
  if (text.length === 0) throw new TypeError('an absolute URL is required');
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

export function originOf(raw) {
  const url = parseUrl(raw);
  return `${url.protocol}//${url.host}`;
}

export function hostOf(raw) {
  return parseUrl(raw).hostname;
}

export function pathOf(raw) {
  const url = parseUrl(raw);
  return `${url.pathname}${url.search}`;
}

export function queryPairs(raw) {
  const url = parseUrl(raw);
  const out = [];
  for (const [key, value] of url.searchParams) out.push([key, value]);
  return out;
}

export function withoutCredentials(raw) {
  const url = parseUrl(raw);
  url.username = '';
  url.password = '';
  return url.href;
}

export function sameOrigin(left, right) {
  return originOf(left) === originOf(right);
}

export function isAbsolute(raw) {
  try {
    parseUrl(raw);
    return true;
  } catch {
    return false;
  }
}

export function joinPath(base, ...parts) {
  const url = parseUrl(base);
  const segments = url.pathname.split('/').filter(Boolean);
  for (const part of parts) {
    segments.push(...String(part ?? '').split('/').filter(Boolean));
  }
  url.pathname = `/${segments.join('/')}`;
  return url.href;
}

export function hostPort(raw) {
  const url = parseUrl(raw);
  return url.port === '' ? url.hostname : `${url.hostname}:${url.port}`;
}
