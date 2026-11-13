const HOST_PATTERN = /^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/;

export function validateEntry(entry) {
  const text = String(entry ?? '').trim().toLowerCase().replace(/\.$/, '');
  if (text.length === 0) throw new TypeError('an allowlist entry cannot be empty');
  if (text.includes('/') || text.includes(':') || text.includes('@')) {
    throw new TypeError(`an allowlist entry is a hostname, not a URL: ${entry}`);
  }
  if (text.includes('*') && !text.startsWith('*.')) {
    throw new TypeError(`a wildcard is only allowed as a leading "*.": ${entry}`);
  }
  if (!HOST_PATTERN.test(text)) throw new TypeError(`not a hostname: ${entry}`);
  return text;
}

export function parseAllowlist(value) {
  if (value === undefined || value === null || value === '') return [];
  const entries = Array.isArray(value) ? value : String(value).split(',');
  const seen = new Set();
  const out = [];
  for (const entry of entries) {
    const text = String(entry ?? '').trim();
    if (text.length === 0) continue;
    const clean = validateEntry(text);
    if (seen.has(clean)) continue;
    seen.add(clean);
    out.push(clean);
  }
  return out;
}

export function matchesEntry(host, entry) {
  const target = String(host ?? '').toLowerCase().replace(/\.$/, '');
  const rule = validateEntry(entry);
  if (rule.startsWith('*.')) {
    const base = rule.slice(2);
    return target === base || target.endsWith(`.${base}`);
  }
  return target === rule || target.endsWith(`.${rule}`);
}

export function isAllowlisted(host, allowlist) {
  return allowlist.some((entry) => matchesEntry(host, entry));
}

export function describeAllowlist(allowlist) {
  if (allowlist.length === 0) return 'any public host';
  return allowlist.join(', ');
}

export function diffAllowlist(before, after) {
  return {
    added: after.filter((entry) => !before.includes(entry)),
    removed: before.filter((entry) => !after.includes(entry)),
  };
}

export function allowlistFromEnv(value) {
  return parseAllowlist(value);
}
