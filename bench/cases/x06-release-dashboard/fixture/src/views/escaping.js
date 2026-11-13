// Two escapers, because two contexts: element text may not carry a tag, and an attribute may not carry a
// quote that closes it. Every attribute in the views goes through escapeAttr (or attrPair, which calls it).
const TEXT_ESCAPES = { '&': '&amp;', '<': '&lt;', '>': '&gt;' };
const ATTR_ESCAPES = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };

const ATTRIBUTE_NAME = /^[a-z][a-z0-9-]*$/i;

export function escapeText(value) {
  return String(value ?? '').replace(/[&<>]/g, (character) => TEXT_ESCAPES[character]);
}

export function escapeAttr(value) {
  return String(value ?? '').replace(/[&<>"']/g, (character) => ATTR_ESCAPES[character]);
}

export function attrPair(name, value) {
  if (typeof name !== 'string' || !ATTRIBUTE_NAME.test(name)) throw new TypeError(`invalid attribute name: ${name}`);
  return `${name}="${escapeAttr(value)}"`;
}

export function element(tag, attributes = {}, body = '') {
  const rendered = Object.entries(attributes)
    .filter(([, value]) => value !== undefined && value !== null && value !== false)
    .map(([name, value]) => (value === true ? name : attrPair(name, value)))
    .join(' ');
  const open = rendered === '' ? `<${tag}>` : `<${tag} ${rendered}>`;
  return `${open}${body}</${tag}>`;
}

export function rawJson(value) {
  return JSON.stringify(value ?? null).replace(/</g, '\\u003c').replace(/>/g, '\\u003e').replace(/&/g, '\\u0026');
}

export function scriptJson(value) {
  return `<script type="application/json">${rawJson(value)}</script>`;
}

export function textBlock(lines) {
  return lines.map((line) => escapeText(line)).join('\n');
}

export function classNames(...values) {
  return values.flat().filter(Boolean).join(' ');
}

export function stripControl(value) {
  return String(value ?? '').replace(/[\u0000-\u001f\u007f]/g, '');
}
