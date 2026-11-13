const TEXT_ESCAPES = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
  '`': '&#96;',
};

export function escapeText(value) {
  return String(value ?? '').replace(/[&<>"'`]/g, (char) => TEXT_ESCAPES[char]);
}

export function escapeAttr(value) {
  return String(value ?? '').replace(/[&<>"'`\u0000-\u001f]/g, (char) => {
    if (char === '\u0000' || char.charCodeAt(0) < 0x20) return '';
    return TEXT_ESCAPES[char];
  });
}

export function escapeJson(value) {
  return JSON.stringify(value ?? null)
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026');
}

export function unescapeText(value) {
  return String(value ?? '')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&amp;/g, '&');
}

export function stripTags(value) {
  return String(value ?? '').replace(/<[^>]*>/g, '');
}

export function safeUrl(value, { schemes = ['https:', 'http:', 'mailto:'] } = {}) {
  const text = String(value ?? '').trim();
  if (text === '') return '';
  const absolute = /^[a-z][a-z0-9+.-]*:/i.test(text);
  if (!absolute) return text.startsWith('/') ? text : '';
  let parsed;
  try {
    parsed = new URL(text);
  } catch {
    return '';
  }
  if (!schemes.includes(parsed.protocol)) return '';
  return text;
}

export function tag(name, attrs = {}, children = '') {
  const rendered = Object.entries(attrs)
    .filter(([, value]) => value !== undefined && value !== null && value !== false)
    .map(([key, value]) => (value === true ? ` ${key}` : ` ${key}="${escapeAttr(value)}"`))
    .join('');
  return `<${name}${rendered}>${children}</${name}>`;
}

export function attrs(map) {
  return Object.entries(map)
    .filter(([, value]) => value !== undefined && value !== null && value !== false)
    .map(([key, value]) => (value === true ? ` ${key}` : ` ${key}="${escapeAttr(value)}"`))
    .join('');
}

export function textNode(value) {
  return escapeText(value);
}

export function cdata(value) {
  return `<![CDATA[${String(value ?? '').replace(/]]>/g, ']]]]><![CDATA[>')}]]>`;
}

export function urlAttr(value) {
  return escapeAttr(safeUrl(value));
}
