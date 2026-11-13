import { singleLine, truncate } from '../util/text.js';
import { imageSize } from './images.js';

export function defaultMetadata(name) {
  const plain = String(name ?? '').replace(/\.[^.]+$/, '').replace(/[-_]+/g, ' ');
  return { title: plain === '' ? 'Untitled' : plain, alt: plain, tags: [], caption: '' };
}

export function normalizeMetadata(input = {}) {
  const title = singleLine(input.title ?? '').slice(0, 160);
  const alt = singleLine(input.alt ?? title).slice(0, 240);
  const caption = truncate(singleLine(input.caption ?? ''), 400);
  const tags = Array.isArray(input.tags)
    ? [...new Set(input.tags.map((tag) => singleLine(tag).toLowerCase()).filter(Boolean))].slice(0, 24)
    : [];
  return { title, alt, caption, tags };
}

export function mergeMetadata(base = {}, patch = {}) {
  const out = { ...normalizeMetadata(base) };
  for (const [key, value] of Object.entries(patch)) {
    if (value === undefined) continue;
    if (key === 'tags' && Array.isArray(value)) {
      out.tags = [...new Set([...(out.tags ?? []), ...normalizeMetadata({ tags: value }).tags])];
      continue;
    }
    if (Object.prototype.hasOwnProperty.call(out, key)) out[key] = value;
  }
  return out;
}

export function describeMedia(name, buffer, meta = {}) {
  const size = imageSize(name, buffer);
  const described = {
    name: String(name),
    bytes: buffer.length,
    ...normalizeMetadata({ ...defaultMetadata(name), ...meta }),
  };
  if (size) described.dimensions = size;
  return described;
}

export function missingAlt(entries) {
  return entries.filter((entry) => entry.alt === undefined || String(entry.alt).trim() === '');
}

export function oversize(entries, limit) {
  return entries.filter((entry) => entry.bytes > limit);
}

export function groupByTag(entries) {
  const out = new Map();
  for (const entry of entries) {
    for (const tag of entry.tags ?? []) {
      const bucket = out.get(tag);
      if (bucket) bucket.push(entry.name);
      else out.set(tag, [entry.name]);
    }
  }
  return out;
}
