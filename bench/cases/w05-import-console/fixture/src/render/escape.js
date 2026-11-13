const TEXT_ESCAPES = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
};

const ATTR_ESCAPES = {
  ...TEXT_ESCAPES,
  '"': '&quot;',
  "'": '&#39;',
  '`': '&#96;',
};

export function escapeText(value) {
  return String(value ?? '').replace(/[&<>]/g, (char) => TEXT_ESCAPES[char]);
}

export function escapeAttr(value) {
  return String(value ?? '').replace(/[&<>"'`]/g, (char) => ATTR_ESCAPES[char]);
}

export function escapeUrl(value) {
  const raw = String(value ?? '').trim();
  if (raw === '') return '#';
  if (/^javascript:/i.test(raw) || /^data:/i.test(raw) || /^vbscript:/i.test(raw)) {
    return '#';
  }
  if (/^(https?:|mailto:|\/|\.\/|#)/i.test(raw)) return raw;
  return '#';
}

export function stripTags(value) {
  return String(value ?? '').replace(/<[^>]*>/g, '');
}

export function jsonForScript(value) {
  return JSON.stringify(value ?? null).replace(/</g, '\\u003c').replace(/\u2028/g, '\\u2028');
}

export function htmlAttributes(attributes) {
  return Object.entries(attributes)
    .filter(([, value]) => value !== undefined && value !== null && value !== false)
    .map(([key, value]) => (value === true ? escapeAttr(key) : `${escapeAttr(key)}="${escapeAttr(value)}"`))
    .join(' ');
}

export function classNames(...names) {
  return names
    .flat()
    .filter((name) => typeof name === 'string' && name.length > 0)
    .map((name) => name.trim())
    .join(' ');
}

export function escapeDeep(value) {
  if (Array.isArray(value)) return value.map((entry) => escapeDeep(entry));
  if (value !== null && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([key, entry]) => [key, escapeDeep(entry)]));
  }
  return escapeText(value);
}
